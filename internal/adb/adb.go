package adb

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"frida-gui-helper/internal/toolchain"
)

type Logger func(level string, source string, message string)

type ToolStatus struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Found   bool   `json:"found"`
	Source  string `json:"source"`
	Version string `json:"version"`
	Error   string `json:"error"`
}

type Device struct {
	Serial       string `json:"serial"`
	State        string `json:"state"`
	Model        string `json:"model"`
	Product      string `json:"product"`
	TransportID  string `json:"transportId"`
	IsAuthorized bool   `json:"isAuthorized"`
}

type AndroidApp struct {
	Package string `json:"package"`
	Path    string `json:"path"`
	Name    string `json:"name"`
	System  bool   `json:"system"`
}

type Process struct {
	PID     int    `json:"pid"`
	User    string `json:"user"`
	Name    string `json:"name"`
	Package string `json:"package"`
}

type FridaServerRequest struct {
	DeviceSerial string
	LocalPath    string
	RemotePath   string
	ForceRestart bool
}

type Runner struct {
	timeout time.Duration
	log     Logger
	tools   *toolchain.Resolver
}

func NewRunner(log Logger, tools *toolchain.Resolver) *Runner {
	if tools == nil {
		tools = toolchain.NewResolver()
	}
	return &Runner{timeout: 30 * time.Second, log: log, tools: tools}
}

func (r *Runner) Status(ctx context.Context) ToolStatus {
	status := ToolStatus{Name: "adb"}
	tool := r.tools.FindExecutable("adb")
	if !tool.Found {
		status.Error = tool.Error
		return status
	}
	status.Found = true
	status.Path = tool.Path
	status.Source = tool.Source

	out, err := r.run(ctx, "version")
	if err != nil {
		status.Error = err.Error()
		return status
	}
	status.Version = firstNonEmptyLine(out)
	return status
}

func (r *Runner) Devices(ctx context.Context) ([]Device, error) {
	out, err := r.run(ctx, "devices", "-l")
	if err != nil {
		return nil, err
	}

	devices := make([]Device, 0)
	for _, line := range strings.Split(normalizeNewlines(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of devices") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		device := Device{
			Serial:       fields[0],
			State:        fields[1],
			IsAuthorized: fields[1] == "device",
		}
		for _, field := range fields[2:] {
			key, value, ok := strings.Cut(field, ":")
			if !ok {
				continue
			}
			switch key {
			case "model":
				device.Model = value
			case "product":
				device.Product = value
			case "transport_id":
				device.TransportID = value
			}
		}
		devices = append(devices, device)
	}

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].Serial < devices[j].Serial
	})
	r.logf("info", "adb", "发现 %d 台设备", len(devices))
	return devices, nil
}

func (r *Runner) ListPackages(ctx context.Context, serial string, includeSystem bool) ([]AndroidApp, error) {
	args := []string{"pm", "list", "packages", "-f"}
	if !includeSystem {
		args = append(args, "-3")
	}
	out, err := r.Shell(ctx, serial, args...)
	if err != nil {
		return nil, err
	}

	apps := make([]AndroidApp, 0)
	for _, line := range strings.Split(normalizeNewlines(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimPrefix(line, "package:")
		separator := strings.LastIndex(line, "=")
		path := ""
		pkg := line
		if separator >= 0 {
			path = line[:separator]
			pkg = line[separator+1:]
		}
		if strings.TrimSpace(pkg) == "" {
			pkg = strings.TrimPrefix(line, "package:")
			path = ""
		}
		app := AndroidApp{
			Package: pkg,
			Path:    path,
			Name:    pkg,
			System:  !strings.HasPrefix(path, "/data/app"),
		}
		apps = append(apps, app)
	}

	sort.Slice(apps, func(i, j int) bool {
		return apps[i].Package < apps[j].Package
	})
	r.logf("info", "adb", "读取到 %d 个应用包", len(apps))
	return apps, nil
}

func (r *Runner) ListProcesses(ctx context.Context, serial string) ([]Process, error) {
	out, err := r.Shell(ctx, serial, "ps", "-A")
	if err != nil {
		out, err = r.Shell(ctx, serial, "ps")
	}
	if err != nil {
		return nil, err
	}

	processes := parseProcesses(out)
	sort.Slice(processes, func(i, j int) bool {
		if processes[i].Name == processes[j].Name {
			return processes[i].PID < processes[j].PID
		}
		return processes[i].Name < processes[j].Name
	})
	r.logf("info", "adb", "读取到 %d 个进程", len(processes))
	return processes, nil
}

func (r *Runner) StartFridaServer(ctx context.Context, req FridaServerRequest) error {
	serial := strings.TrimSpace(req.DeviceSerial)
	if serial == "" {
		return errors.New("device serial is required")
	}

	remotePath := strings.TrimSpace(req.RemotePath)
	if remotePath == "" {
		remotePath = "/data/local/tmp/frida-server"
	}
	if !isSafeRemotePath(remotePath) {
		return fmt.Errorf("remote frida-server path is not safe: %s", remotePath)
	}

	if !req.ForceRestart {
		pidOut, _ := r.FridaServerPID(ctx, serial)
		if pidOut != "" {
			r.logf("info", "frida-server", "设备上已有 frida-server 进程: %s", pidOut)
			return nil
		}
	}

	localPath := strings.TrimSpace(req.LocalPath)
	if localPath != "" {
		if _, err := os.Stat(localPath); err != nil {
			return fmt.Errorf("读取本地 frida-server 失败: %w", err)
		}
		if _, err := r.run(ctx, "-s", serial, "push", localPath, remotePath); err != nil {
			return err
		}
		r.logf("info", "frida-server", "已推送 frida-server 到 %s", remotePath)
	} else if _, err := r.Shell(ctx, serial, "ls", remotePath); err != nil {
		return fmt.Errorf("设备上未找到 %s，请填写本地 frida-server 路径后重试", remotePath)
	}

	if _, err := r.Shell(ctx, serial, "chmod", "755", remotePath); err != nil {
		return err
	}

	if req.ForceRestart {
		_, _ = r.StopFridaServer(ctx, serial)
		time.Sleep(500 * time.Millisecond)
	}

	if err := r.StartRemoteFridaServer(ctx, serial, remotePath); err != nil {
		return err
	}

	time.Sleep(700 * time.Millisecond)
	pidOut, _ := r.FridaServerPID(ctx, serial)
	if pidOut == "" {
		return errors.New("frida-server 启动后未检测到进程，请确认设备 Root 权限和二进制架构")
	}
	r.logf("info", "frida-server", "frida-server 已启动: %s", pidOut)
	return nil
}

func (r *Runner) StartRemoteFridaServer(ctx context.Context, serial string, remotePath string) error {
	remotePath = strings.TrimSpace(remotePath)
	if remotePath == "" {
		remotePath = "/data/local/tmp/frida-server"
	}
	if !isSafeRemotePath(remotePath) {
		return fmt.Errorf("remote frida-server path is not safe: %s", remotePath)
	}

	if pid, _ := r.FridaServerPID(ctx, serial); pid != "" {
		r.logf("info", "frida-server", "frida-server 已在运行: %s", pid)
		return nil
	}

	if _, err := r.Shell(ctx, serial, "ls", remotePath); err != nil {
		return fmt.Errorf("设备上未找到 %s，请先推送 frida-server", remotePath)
	}
	if _, err := r.Shell(ctx, serial, "chmod", "755", remotePath); err != nil {
		return err
	}

	logPath := "/data/local/tmp/frida-server.log"
	startCommand := fmt.Sprintf("rm -f %s; nohup %s --verbose >%s 2>&1 &", logPath, remotePath, logPath)
	attempts := []struct {
		label   string
		command string
	}{
		{"su -c", "su -c " + shellQuote(startCommand)},
		{"su 0 sh -c", "su 0 sh -c " + shellQuote(startCommand)},
		{"direct shell", "sh -c " + shellQuote(startCommand)},
	}

	var lastErr error
	for _, attempt := range attempts {
		out, err := r.ShellCommandQuiet(ctx, serial, attempt.command)
		if err == nil {
			r.logf("info", "frida-server", "启动命令已发送: %s", attempt.label)
			if strings.TrimSpace(out) != "" {
				r.logf("info", "frida-server", strings.TrimSpace(out))
			}
			return nil
		}
		lastErr = err
		r.logf("warn", "frida-server", "%s 启动失败: %v", attempt.label, err)
	}
	return fmt.Errorf("启动 frida-server 失败: %w", lastErr)
}

func (r *Runner) FridaServerPID(ctx context.Context, serial string) (string, error) {
	out, err := r.ShellQuiet(ctx, serial, "pidof", "frida-server")
	out = strings.TrimSpace(out)
	if out == "" && err != nil {
		return "", nil
	}
	return out, err
}

func (r *Runner) StopFridaServer(ctx context.Context, serial string) (string, error) {
	pid, _ := r.FridaServerPID(ctx, serial)
	if pid == "" {
		return "未检测到 frida-server 进程", nil
	}

	killCommand := "kill -9 " + pid
	commands := []string{
		"su -c " + shellQuote(killCommand),
		"su 0 sh -c " + shellQuote(killCommand),
		killCommand,
		"su -c " + shellQuote("pkill -9 -f frida-server"),
		"su 0 sh -c " + shellQuote("pkill -9 -f frida-server"),
		"pkill -9 -f frida-server",
	}

	var lastErr error
	for _, command := range commands {
		_, err := r.ShellCommandQuiet(ctx, serial, command)
		time.Sleep(250 * time.Millisecond)
		if currentPID, _ := r.FridaServerPID(ctx, serial); currentPID == "" {
			return "已停止 frida-server: " + pid, nil
		}
		if err == nil {
			lastErr = nil
			continue
		}
		lastErr = err
	}
	if lastErr != nil {
		return "", fmt.Errorf("停止 frida-server 失败，PID %s 仍在运行: %w", pid, lastErr)
	}
	return "", fmt.Errorf("停止 frida-server 失败，PID %s 仍在运行", pid)
}

func (r *Runner) Shell(ctx context.Context, serial string, args ...string) (string, error) {
	adbArgs := make([]string, 0, len(args)+3)
	if strings.TrimSpace(serial) != "" {
		adbArgs = append(adbArgs, "-s", serial)
	}
	adbArgs = append(adbArgs, "shell")
	adbArgs = append(adbArgs, args...)
	return r.run(ctx, adbArgs...)
}

func (r *Runner) ShellQuiet(ctx context.Context, serial string, args ...string) (string, error) {
	adbArgs := make([]string, 0, len(args)+3)
	if strings.TrimSpace(serial) != "" {
		adbArgs = append(adbArgs, "-s", serial)
	}
	adbArgs = append(adbArgs, "shell")
	adbArgs = append(adbArgs, args...)
	return r.runInternal(ctx, false, adbArgs...)
}

func (r *Runner) ShellCommand(ctx context.Context, serial string, command string) (string, error) {
	adbArgs := make([]string, 0, 4)
	if strings.TrimSpace(serial) != "" {
		adbArgs = append(adbArgs, "-s", serial)
	}
	adbArgs = append(adbArgs, "shell", command)
	return r.run(ctx, adbArgs...)
}

func (r *Runner) ShellCommandQuiet(ctx context.Context, serial string, command string) (string, error) {
	adbArgs := make([]string, 0, 4)
	if strings.TrimSpace(serial) != "" {
		adbArgs = append(adbArgs, "-s", serial)
	}
	adbArgs = append(adbArgs, "shell", command)
	return r.runInternal(ctx, false, adbArgs...)
}

func (r *Runner) Forward(ctx context.Context, serial string, local string, remote string) (string, error) {
	adbArgs := make([]string, 0, 5)
	if strings.TrimSpace(serial) != "" {
		adbArgs = append(adbArgs, "-s", serial)
	}
	adbArgs = append(adbArgs, "forward", local, remote)
	return r.run(ctx, adbArgs...)
}

func (r *Runner) run(ctx context.Context, args ...string) (string, error) {
	return r.runInternal(ctx, true, args...)
}

func (r *Runner) runInternal(ctx context.Context, logError bool, args ...string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmdCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	adbTool := r.tools.FindExecutable("adb")
	if !adbTool.Found {
		return "", fmt.Errorf("adb not found: %s", adbTool.Error)
	}

	cmd := exec.CommandContext(cmdCtx, adbTool.Path, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = err.Error()
		}
		if logError {
			r.logf("error", "adb", message)
		}
		return strings.TrimSpace(stdout.String()), fmt.Errorf("adb %s: %s", strings.Join(args, " "), message)
	}

	return stdout.String(), nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func parseProcesses(out string) []Process {
	lines := strings.Split(normalizeNewlines(out), "\n")
	if len(lines) == 0 {
		return []Process{}
	}

	headerIndex := -1
	for i, line := range lines {
		if strings.Contains(line, "PID") {
			headerIndex = i
			break
		}
	}
	if headerIndex < 0 {
		return []Process{}
	}

	header := strings.Fields(lines[headerIndex])
	pidIndex := indexAny(header, "PID")
	userIndex := indexAny(header, "USER", "UID")
	nameIndex := indexAny(header, "NAME", "ARGS", "CMDLINE", "CMD")
	if pidIndex < 0 {
		return []Process{}
	}
	if nameIndex < 0 {
		nameIndex = len(header) - 1
	}

	processes := make([]Process, 0)
	for _, line := range lines[headerIndex+1:] {
		fields := strings.Fields(line)
		if len(fields) <= pidIndex || len(fields) <= nameIndex {
			continue
		}

		pid, err := strconv.Atoi(fields[pidIndex])
		if err != nil {
			continue
		}

		user := ""
		if userIndex >= 0 && len(fields) > userIndex {
			user = fields[userIndex]
		}
		name := strings.Join(fields[nameIndex:], " ")
		processes = append(processes, Process{
			PID:     pid,
			User:    user,
			Name:    name,
			Package: packageFromProcessName(name),
		})
	}
	return processes
}

func packageFromProcessName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if strings.Contains(name, " ") {
		name = strings.Fields(name)[0]
	}
	if strings.Contains(name, "/") {
		parts := strings.Split(name, "/")
		name = parts[len(parts)-1]
	}
	if before, _, ok := strings.Cut(name, ":"); ok && strings.Contains(before, ".") {
		return before
	}
	if strings.Contains(name, ".") {
		return name
	}
	return ""
}

func indexAny(values []string, keys ...string) int {
	for i, value := range values {
		for _, key := range keys {
			if strings.EqualFold(value, key) {
				return i
			}
		}
	}
	return -1
}

func firstNonEmptyLine(text string) string {
	for _, line := range strings.Split(normalizeNewlines(text), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func normalizeNewlines(text string) string {
	return strings.ReplaceAll(text, "\r\n", "\n")
}

func isSafeRemotePath(path string) bool {
	if !strings.HasPrefix(path, "/") {
		return false
	}
	for _, r := range path {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		switch r {
		case '/', '.', '_', '-':
			continue
		default:
			return false
		}
	}
	return true
}

func (r *Runner) logf(level string, source string, format string, args ...interface{}) {
	if r.log == nil {
		return
	}
	r.log(level, source, fmt.Sprintf(format, args...))
}
