package frida

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

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

type RunRequest struct {
	DeviceSerial string
	Mode         string
	TargetKind   string
	Target       string
	ScriptName   string
	ScriptSource string
}

type SessionInfo struct {
	ID           string `json:"id"`
	DeviceSerial string `json:"deviceSerial"`
	Mode         string `json:"mode"`
	TargetKind   string `json:"targetKind"`
	Target       string `json:"target"`
	ScriptName   string `json:"scriptName"`
	StartedAt    string `json:"startedAt"`
	Running      bool   `json:"running"`
}

type session struct {
	info       SessionInfo
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	stdin      io.WriteCloser
	scriptPath string
}

type Runner struct {
	log      Logger
	tools    *toolchain.Resolver
	mu       sync.Mutex
	sessions map[string]*session
}

func NewRunner(log Logger, tools *toolchain.Resolver) *Runner {
	if tools == nil {
		tools = toolchain.NewResolver()
	}
	return &Runner{
		log:      log,
		tools:    tools,
		sessions: make(map[string]*session),
	}
}

func (r *Runner) Status(ctx context.Context) ToolStatus {
	status := ToolStatus{Name: "frida"}
	tool := r.tools.FindExecutable("frida")
	if !tool.Found {
		status.Error = tool.Error
		return status
	}
	status.Found = true
	status.Path = tool.Path
	status.Source = tool.Source

	cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, tool.Path, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		status.Error = strings.TrimSpace(string(out))
		if status.Error == "" {
			status.Error = err.Error()
		}
		return status
	}
	status.Version = strings.TrimSpace(string(out))
	return status
}

func (r *Runner) RunScript(ctx context.Context, req RunRequest) (SessionInfo, error) {
	req.Mode = normalizeMode(req.Mode)
	req.TargetKind = normalizeTargetKind(req.TargetKind)
	req.Target = strings.TrimSpace(req.Target)
	req.ScriptSource = strings.TrimSpace(req.ScriptSource)
	if req.Target == "" {
		return SessionInfo{}, errors.New("target is required")
	}
	if req.ScriptSource == "" {
		return SessionInfo{}, errors.New("script source is required")
	}

	scriptPath, err := writeTempScript(req.ScriptSource)
	if err != nil {
		return SessionInfo{}, err
	}

	args, err := buildArgs(req, scriptPath)
	if err != nil {
		_ = os.Remove(scriptPath)
		return SessionInfo{}, err
	}

	sessionID := newID()
	cmdCtx, cancel := context.WithCancel(ctx)
	fridaTool := r.tools.FindExecutable("frida")
	if !fridaTool.Found {
		cancel()
		_ = os.Remove(scriptPath)
		return SessionInfo{}, fmt.Errorf("frida not found: %s", fridaTool.Error)
	}
	cmd := exec.CommandContext(cmdCtx, fridaTool.Path, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		_ = os.Remove(scriptPath)
		return SessionInfo{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		_ = os.Remove(scriptPath)
		return SessionInfo{}, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		_ = os.Remove(scriptPath)
		return SessionInfo{}, err
	}

	if err := cmd.Start(); err != nil {
		cancel()
		_ = os.Remove(scriptPath)
		return SessionInfo{}, err
	}

	info := SessionInfo{
		ID:           sessionID,
		DeviceSerial: req.DeviceSerial,
		Mode:         req.Mode,
		TargetKind:   req.TargetKind,
		Target:       req.Target,
		ScriptName:   displayScriptName(req.ScriptName),
		StartedAt:    time.Now().Format(time.RFC3339),
		Running:      true,
	}

	r.mu.Lock()
	r.sessions[sessionID] = &session{
		info:       info,
		cmd:        cmd,
		cancel:     cancel,
		stdin:      stdin,
		scriptPath: scriptPath,
	}
	r.mu.Unlock()

	r.logf("info", "frida", "启动会话 %s: frida %s", sessionID, strings.Join(args, " "))
	go r.stream(sessionID, "stdout", stdout, "info")
	go r.stream(sessionID, "stderr", stderr, "error")
	go r.wait(sessionID, cmd, scriptPath)

	return info, nil
}

func (r *Runner) StopSession(sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id is required")
	}

	r.mu.Lock()
	sess, ok := r.sessions[sessionID]
	if ok {
		delete(r.sessions, sessionID)
	}
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}

	sess.cancel()
	_ = sess.stdin.Close()
	if sess.cmd != nil && sess.cmd.Process != nil {
		_ = sess.cmd.Process.Kill()
	}
	_ = os.Remove(sess.scriptPath)
	r.logf("info", "frida", "已停止会话 %s", sessionID)
	return nil
}

func (r *Runner) StopAll() {
	for _, info := range r.ListSessions() {
		_ = r.StopSession(info.ID)
	}
}

func (r *Runner) ListSessions() []SessionInfo {
	r.mu.Lock()
	defer r.mu.Unlock()

	sessions := make([]SessionInfo, 0, len(r.sessions))
	for _, sess := range r.sessions {
		sessions = append(sessions, sess.info)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartedAt > sessions[j].StartedAt
	})
	return sessions
}

func (r *Runner) wait(sessionID string, cmd *exec.Cmd, scriptPath string) {
	err := cmd.Wait()

	var stdin io.WriteCloser
	r.mu.Lock()
	if sess, ok := r.sessions[sessionID]; ok {
		sess.info.Running = false
		stdin = sess.stdin
	}
	r.mu.Unlock()
	if stdin != nil {
		_ = stdin.Close()
	}
	_ = os.Remove(scriptPath)

	if err != nil {
		r.logf("error", "frida", "会话 %s 已退出: %v", sessionID, err)
		return
	}
	r.logf("info", "frida", "会话 %s 已退出", sessionID)
}

func (r *Runner) stream(sessionID string, pipeName string, reader io.Reader, level string) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		r.logf(level, "frida:"+sessionID+":"+pipeName, line)
		if strings.Contains(line, "need Gadget to attach on jailed Android") {
			r.logf("error", "frida", "未连接到设备端 frida-server。请下载与本机 Frida CLI 完全一致版本和设备架构匹配的 frida-server，推送到 /data/local/tmp/frida-server 后启动。")
		}
		if strings.Contains(line, "unexpected early end-of-stream") {
			r.logf("error", "frida", "Frida 在脚本执行前断开。请先运行“Frida 官方 smoke test”和“检查 server 身份/版本”；如果 smoke test 正常，再用“连接探针”模板区分目标 App 启动崩溃、反调试/反 Frida、或脚本逻辑问题。")
		}
	}
	if err := scanner.Err(); err != nil {
		r.logf("error", "frida:"+sessionID, err.Error())
	}
}

func buildArgs(req RunRequest, scriptPath string) ([]string, error) {
	args := make([]string, 0, 8)
	if strings.TrimSpace(req.DeviceSerial) != "" {
		args = append(args, "-D", strings.TrimSpace(req.DeviceSerial))
	} else {
		args = append(args, "-U")
	}

	switch req.Mode {
	case "spawn":
		if req.TargetKind != "package" && req.TargetKind != "name" {
			return nil, errors.New("spawn mode requires package/name target")
		}
		args = append(args, "-f", req.Target)
	case "attach":
		switch req.TargetKind {
		case "pid":
			args = append(args, "-p", req.Target)
		case "package":
			args = append(args, "-N", req.Target)
		default:
			args = append(args, "-n", req.Target)
		}
	default:
		return nil, fmt.Errorf("unsupported mode: %s", req.Mode)
	}

	args = append(args, "-l", scriptPath)
	return args, nil
}

func normalizeMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "spawn":
		return "spawn"
	default:
		return "attach"
	}
}

func normalizeTargetKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "pid":
		return "pid"
	case "package":
		return "package"
	default:
		return "name"
	}
}

func writeTempScript(source string) (string, error) {
	dir := filepath.Join(os.TempDir(), "frida-gui-helper")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "hook-"+newID()+".js")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func displayScriptName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "custom-script"
	}
	return name
}

func newID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func (r *Runner) logf(level string, source string, format string, args ...interface{}) {
	if r.log == nil {
		return
	}
	r.log(level, source, fmt.Sprintf(format, args...))
}
