package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"frida-gui-helper/internal/adb"
)

const (
	preflightPass = "pass"
	preflightFail = "fail"
)

type PreflightRequest struct {
	DeviceSerial string `json:"deviceSerial"`
	Mode         string `json:"mode"`
	TargetKind   string `json:"targetKind"`
	Target       string `json:"target"`
	ServerPath   string `json:"serverPath"`
}

type PreflightCheck struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Action  string `json:"action"`
}

type PreflightResult struct {
	Ready           bool             `json:"ready"`
	DeviceSerial    string           `json:"deviceSerial"`
	ABI             string           `json:"abi"`
	CLIVersion      string           `json:"cliVersion"`
	ServerVersion   string           `json:"serverVersion"`
	ServerPath      string           `json:"serverPath"`
	TargetAvailable bool             `json:"targetAvailable"`
	CheckedAt       string           `json:"checkedAt"`
	Checks          []PreflightCheck `json:"checks"`
}

// RunPreflight validates the complete device-to-target injection path without
// mutating the device. Failures are returned as individual checks so the UI can
// present an actionable result instead of one opaque error.
func (a *App) RunPreflight(req PreflightRequest) PreflightResult {
	ctx, cancel := a.withTimeout(35 * time.Second)
	defer cancel()
	return a.runPreflight(ctx, req)
}

// RestartFridaServerAndCheck is the one-click repair action used by preflight.
// It always uses the application-managed binary and then reruns every check.
func (a *App) RestartFridaServerAndCheck(req PreflightRequest) (PreflightResult, error) {
	serial := strings.TrimSpace(req.DeviceSerial)
	if serial == "" {
		return a.RunPreflight(req), fmt.Errorf("请先选择 Android 设备")
	}
	remotePath := strings.TrimSpace(req.ServerPath)
	if remotePath == "" {
		remotePath = adb.DefaultFridaServerRemotePath
	}
	if err := a.StartFridaServer(FridaServerRequest{
		DeviceSerial: serial,
		RemotePath:   remotePath,
		ForceRestart: true,
	}); err != nil {
		return a.RunPreflight(req), err
	}
	return a.RunPreflight(req), nil
}

func (a *App) runPreflight(ctx context.Context, req PreflightRequest) PreflightResult {
	serial := strings.TrimSpace(req.DeviceSerial)
	mode := normalizePreflightMode(req.Mode)
	targetKind := normalizePreflightTargetKind(req.TargetKind)
	target := strings.TrimSpace(req.Target)
	result := PreflightResult{
		DeviceSerial: serial,
		CheckedAt:    time.Now().Format(time.RFC3339),
		Checks:       make([]PreflightCheck, 0, 7),
	}

	device, deviceErr := a.preflightDevice(ctx, serial)
	if deviceErr != nil {
		result.addCheck("device", "设备与 ADB 授权", preflightFail, deviceErr.Error(), "连接设备、开启 USB 调试并在手机上允许此电脑")
		result.addUnavailableChecks()
		return result
	}
	result.addCheck("device", "设备与 ADB 授权", preflightPass, device.Serial+" · "+firstNonEmpty(device.Model, device.Product, "Android device"), "")

	rootIdentity, rootErr := a.preflightRoot(ctx, serial)
	if rootErr != nil {
		result.addCheck("root", "Root 权限", preflightFail, rootErr.Error(), "在 Magisk、KernelSU 或 APatch 中授予 Shell/ADB shell 超级用户权限")
	} else {
		result.addCheck("root", "Root 权限", preflightPass, rootIdentity, "")
	}

	abi, abiErr := a.preflightABI(ctx, serial)
	if abiErr != nil {
		result.addCheck("abi", "设备 ABI", preflightFail, abiErr.Error(), "确认设备系统可正常执行 getprop")
	} else {
		result.ABI = abi
		result.addCheck("abi", "设备 ABI", preflightPass, abi, "")
	}

	cli := a.frida.Status(ctx)
	result.CLIVersion = strings.TrimSpace(cli.Version)
	if !cli.Found || result.CLIVersion == "" {
		message := firstNonEmpty(strings.TrimSpace(cli.Error), "安装包内 Frida CLI 不可用")
		result.addCheck("cli", "内置 Frida CLI", preflightFail, message, "在设置中检查或更新 Frida 组件")
	} else {
		result.addCheck("cli", "内置 Frida CLI", preflightPass, result.CLIVersion+" · "+cli.Source, "")
	}

	if abiErr == nil {
		bundled := a.runtime.BundledStatus(ctx, abi)
		if !bundled.Server.Found {
			result.addCheck("server-binary", "匹配 ABI 的 server", preflightFail, firstNonEmpty(bundled.Server.Error, "安装包内没有匹配的 frida-server"), "在设置中检查或更新 Frida 组件")
		} else {
			result.addCheck("server-binary", "匹配 ABI 的 server", preflightPass, bundled.Server.Path, "")
		}
	} else {
		result.addCheck("server-binary", "匹配 ABI 的 server", preflightFail, "未读取到 ABI，无法选择 server", "先修复设备 ABI 检查")
	}

	server := a.getFridaServerStatus(ctx, serial)
	result.ServerVersion = strings.TrimSpace(server.ServerVersion)
	result.ServerPath = strings.TrimSpace(server.Path)
	if server.State != "running" || !server.VersionMatch {
		message := firstNonEmpty(strings.TrimSpace(server.Error), "frida-server 未运行")
		if server.State == "running" && result.CLIVersion != "" && result.ServerVersion == "" {
			message = "server 正在运行，但无法读取版本"
		}
		result.addCheck("server", "frida-server 状态与版本", preflightFail, message, "点击“一键启动/重启 server”推送并启动同版本 server")
	} else {
		result.addCheck("server", "frida-server 状态与版本", preflightPass, fmt.Sprintf("%s · PID %s", result.ServerVersion, server.PID), "")
	}

	targetOK, targetMessage := a.preflightTarget(ctx, serial, mode, targetKind, target)
	result.TargetAvailable = targetOK
	if targetOK {
		result.addCheck("target", "目标应用或进程", preflightPass, targetMessage, "")
	} else {
		action := "确认包名/进程名/PID；Attach 模式需先启动目标进程"
		if mode == "spawn" {
			action = "确认目标包已安装，或从“进程 → 应用包”中选择"
		}
		result.addCheck("target", "目标应用或进程", preflightFail, targetMessage, action)
	}

	result.Ready = true
	for _, check := range result.Checks {
		if check.Status == preflightFail {
			result.Ready = false
			break
		}
	}
	return result
}

func (a *App) preflightDevice(ctx context.Context, serial string) (adb.Device, error) {
	if serial == "" {
		return adb.Device{}, fmt.Errorf("尚未选择设备")
	}
	devices, err := a.adb.Devices(ctx)
	if err != nil {
		return adb.Device{}, fmt.Errorf("读取设备失败: %w", err)
	}
	for _, device := range devices {
		if device.Serial != serial {
			continue
		}
		if !device.IsAuthorized {
			return adb.Device{}, fmt.Errorf("设备状态为 %s，尚未授权", firstNonEmpty(device.State, "unknown"))
		}
		return device, nil
	}
	return adb.Device{}, fmt.Errorf("ADB 列表中没有设备 %s", serial)
}

func (a *App) preflightRoot(ctx context.Context, serial string) (string, error) {
	for _, command := range []string{"su -c 'id'", "su 0 id"} {
		out, err := a.adb.ShellCommandQuiet(ctx, serial, command)
		out = strings.TrimSpace(out)
		if err == nil && strings.Contains(out, "uid=0") {
			return firstNonEmpty(out, "uid=0(root)"), nil
		}
	}
	identity, _ := a.adb.ShellCommandQuiet(ctx, serial, "id")
	if identity = strings.TrimSpace(identity); identity != "" {
		return "", fmt.Errorf("当前 shell 不是 Root: %s", identity)
	}
	return "", fmt.Errorf("无法获得 Root shell")
}

func (a *App) preflightABI(ctx context.Context, serial string) (string, error) {
	for _, property := range []string{"ro.product.cpu.abilist", "ro.product.cpu.abi"} {
		out, err := a.adb.ShellQuiet(ctx, serial, "getprop", property)
		if err != nil {
			continue
		}
		if abi := strings.TrimSpace(out); abi != "" {
			return abi, nil
		}
	}
	return "", fmt.Errorf("无法读取设备 ABI")
}

func (a *App) preflightTarget(ctx context.Context, serial, mode, kind, target string) (bool, string) {
	if target == "" {
		return false, "尚未填写目标"
	}
	if mode == "spawn" {
		out, err := a.adb.ShellQuiet(ctx, serial, "pm", "path", target)
		if err != nil || !strings.Contains(out, "package:") {
			return false, "设备上未找到应用包 " + target
		}
		return true, "应用包已安装: " + target
	}

	processes, err := a.adb.ListProcesses(ctx, serial)
	if err != nil {
		return false, "读取目标进程失败: " + err.Error()
	}
	for _, process := range processes {
		if processMatchesTarget(process, kind, target) {
			return true, fmt.Sprintf("进程已运行: %s (PID %d)", firstNonEmpty(process.Name, process.Package), process.PID)
		}
	}
	return false, "未找到正在运行的目标 " + target
}

func processMatchesTarget(process adb.Process, kind, target string) bool {
	switch kind {
	case "pid":
		pid, err := strconv.Atoi(target)
		return err == nil && process.PID == pid
	case "package":
		return process.Package == target || process.Name == target || strings.HasPrefix(process.Name, target+":")
	default:
		return process.Name == target || process.Package == target
	}
}

func (r *PreflightResult) addCheck(id, label, status, message, action string) {
	r.Checks = append(r.Checks, PreflightCheck{ID: id, Label: label, Status: status, Message: message, Action: action})
}

func (r *PreflightResult) addUnavailableChecks() {
	for _, check := range []struct{ id, label string }{
		{"root", "Root 权限"},
		{"abi", "设备 ABI"},
		{"cli", "内置 Frida CLI"},
		{"server-binary", "匹配 ABI 的 server"},
		{"server", "frida-server 状态与版本"},
		{"target", "目标应用或进程"},
	} {
		r.addCheck(check.id, check.label, preflightFail, "设备不可用，未执行此项", "先修复设备连接与授权")
	}
}

func normalizePreflightMode(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "spawn") {
		return "spawn"
	}
	return "attach"
}

func normalizePreflightTargetKind(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "pid":
		return "pid"
	case "package":
		return "package"
	default:
		return "name"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
