package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"frida-gui-helper/internal/adb"
	"frida-gui-helper/internal/diagnostics"
	"frida-gui-helper/internal/logstream"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const AppVersion = "1.0.5"

type FridaServerStatus struct {
	State         string `json:"state"`
	PID           string `json:"pid"`
	User          string `json:"user"`
	Path          string `json:"path"`
	ServerVersion string `json:"serverVersion"`
	CLIVersion    string `json:"cliVersion"`
	VersionMatch  bool   `json:"versionMatch"`
	CheckedAt     string `json:"checkedAt"`
	Error         string `json:"error"`
}

type diagnosticReport struct {
	AppVersion   string            `json:"appVersion"`
	GeneratedAt  string            `json:"generatedAt"`
	System       SystemStatus      `json:"system"`
	Server       FridaServerStatus `json:"server"`
	DeviceSerial string            `json:"deviceSerial"`
	Android      map[string]string `json:"android"`
}

func (a *App) GetFridaServerStatus(deviceSerial string) FridaServerStatus {
	ctx, cancel := a.withTimeout(12 * time.Second)
	defer cancel()
	return a.getFridaServerStatus(ctx, strings.TrimSpace(deviceSerial))
}

func (a *App) getFridaServerStatus(ctx context.Context, serial string) FridaServerStatus {
	status := FridaServerStatus{
		State:     "unknown",
		CheckedAt: time.Now().Format(time.RFC3339),
	}
	if serial == "" {
		status.State = "unavailable"
		status.Error = "请先选择 Android 设备"
		return status
	}

	if _, err := a.adb.ShellQuiet(ctx, serial, "echo", "ok"); err != nil {
		status.State = "unavailable"
		status.Error = err.Error()
		return status
	}

	cli := a.frida.Status(ctx)
	status.CLIVersion = strings.TrimSpace(cli.Version)
	pid, err := a.adb.FridaServerPID(ctx, serial)
	if err != nil || strings.TrimSpace(pid) == "" {
		status.State = "stopped"
		if cli.Error != "" {
			status.Error = cli.Error
		}
		return status
	}

	status.State = "running"
	status.PID = strings.TrimSpace(pid)
	process, _ := a.adb.ShellCommandQuiet(ctx, serial, "ps -A | grep '[f]rida-server'")
	status.User = firstField(process)
	path, version, versionErr := a.adb.InspectFridaServerVersion(ctx, serial, "")
	status.Path = strings.TrimSpace(path)
	status.ServerVersion = strings.TrimSpace(version)
	status.VersionMatch = status.CLIVersion != "" && status.ServerVersion != "" && status.CLIVersion == status.ServerVersion
	if versionErr != nil {
		status.State = "degraded"
		status.Error = versionErr.Error()
	} else if status.CLIVersion != "" && status.ServerVersion != "" && !status.VersionMatch {
		status.State = "mismatch"
		status.Error = fmt.Sprintf("CLI %s 与 server %s 不一致", status.CLIVersion, status.ServerVersion)
	}
	return status
}

func (a *App) ExportDiagnosticBundle(deviceSerial string) (string, error) {
	serial := strings.TrimSpace(deviceSerial)
	defaultName := "frida-gui-helper-diagnostics-" + time.Now().Format("20060102-150405") + ".zip"
	path, err := wailsRuntime.SaveFileDialog(a.baseContext(), wailsRuntime.SaveDialogOptions{
		Title:           "导出诊断包",
		DefaultFilename: defaultName,
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "ZIP archive (*.zip)", Pattern: "*.zip"},
		},
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	if !strings.EqualFold(filepath.Ext(path), ".zip") {
		path += ".zip"
	}

	ctx, cancel := a.withTimeout(25 * time.Second)
	defer cancel()
	report := diagnosticReport{
		AppVersion:   AppVersion,
		GeneratedAt:  time.Now().Format(time.RFC3339),
		System:       a.GetSystemStatus(),
		Server:       a.getFridaServerStatus(ctx, serial),
		DeviceSerial: serial,
		Android:      map[string]string{},
	}
	serverLog := "未选择设备，未采集 frida-server 日志。"
	logcat := "未选择设备，未采集 logcat。"
	if serial != "" {
		report.Android["release"], _ = a.adb.ShellQuiet(ctx, serial, "getprop", "ro.build.version.release")
		report.Android["sdk"], _ = a.adb.ShellQuiet(ctx, serial, "getprop", "ro.build.version.sdk")
		report.Android["abi"], _ = a.adb.ShellQuiet(ctx, serial, "getprop", "ro.product.cpu.abi")
		report.Android["selinux"], _ = a.adb.ShellQuiet(ctx, serial, "getenforce")
		serverLog = collectDiagnosticCommand(func() (string, error) {
			return a.adb.ShellQuiet(ctx, serial, "tail", "-n", "300", adb.FridaServerLogPath)
		})
		logcat = collectDiagnosticCommand(func() (string, error) {
			return a.adb.ShellCommandQuiet(ctx, serial, "logcat -d -t 800 | grep -i -E 'frida|gum|ptrace|avc|denied|crash|fatal|anr|debugger|zygote'")
		})
	}

	secrets := diagnosticSecrets(serial, report.System)
	reportJSON, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	logsText := formatDiagnosticLogs(a.logs.Entries())

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	if err := addZipText(writer, "report.json", diagnostics.Sanitize(string(reportJSON), secrets...)); err != nil {
		return "", err
	}
	if err := addZipText(writer, "logs.txt", diagnostics.Sanitize(logsText, secrets...)); err != nil {
		return "", err
	}
	if err := addZipText(writer, "frida-server.log", diagnostics.Sanitize(serverLog, secrets...)); err != nil {
		return "", err
	}
	if err := addZipText(writer, "logcat-frida.txt", diagnostics.Sanitize(logcat, secrets...)); err != nil {
		return "", err
	}
	if err := addZipText(writer, "README.txt", "This diagnostic bundle was generated by Frida GUI Helper.\r\nLocal paths, user names and the selected device serial are sanitized.\r\nNo script source is included.\r\n"); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, buffer.Bytes(), 0o600); err != nil {
		return "", err
	}
	a.addLog("info", "diagnostic", "诊断包已导出: "+filepath.Base(path))
	return path, nil
}

func diagnosticSecrets(serial string, system SystemStatus) []string {
	values := []string{serial, system.ADB.Path, system.Frida.Path, system.Python.Path}
	for _, lookup := range []func() (string, error){os.UserHomeDir, os.UserConfigDir, os.Getwd} {
		if value, err := lookup(); err == nil {
			values = append(values, value)
		}
	}
	values = append(values, os.TempDir())
	if executable, err := os.Executable(); err == nil {
		values = append(values, filepath.Dir(executable))
	}
	return values
}

func formatDiagnosticLogs(entries []logstream.Entry) string {
	var builder strings.Builder
	for _, entry := range entries {
		fmt.Fprintf(&builder, "[%s] %s %s: %s\n", entry.Time, strings.ToUpper(string(entry.Level)), entry.Source, entry.Message)
		if entry.Diagnostic != nil {
			fmt.Fprintf(&builder, "  diagnostic=%s action=%s\n", entry.Diagnostic.Code, entry.Diagnostic.Action)
		}
	}
	return builder.String()
}

func addZipText(writer *zip.Writer, name string, content string) error {
	if writer == nil {
		return errors.New("zip writer is nil")
	}
	entry, err := writer.Create(name)
	if err != nil {
		return err
	}
	_, err = entry.Write([]byte(content))
	return err
}

func firstField(value string) string {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func collectDiagnosticCommand(command func() (string, error)) string {
	output, err := command()
	output = strings.TrimSpace(output)
	if err != nil {
		if output != "" {
			return err.Error() + "\n" + output
		}
		return err.Error()
	}
	if output == "" {
		return "未读取到相关日志。"
	}
	return output
}
