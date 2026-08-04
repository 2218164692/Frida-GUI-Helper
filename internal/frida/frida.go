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

	"frida-gui-helper/internal/processutil"
	"frida-gui-helper/internal/toolchain"
)

type Logger func(level string, source string, message string)
type SessionEmitter func(SessionInfo)
type SessionLogEmitter func(SessionLogEntry)

const (
	defaultMaxSessionHistory      = 50
	defaultMaxLogsPerSession      = 2000
	defaultMaxLogBytesPerSession  = 4 * 1024 * 1024
	defaultMaxSessionLogLineBytes = 16 * 1024
)

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
	EndedAt      string `json:"endedAt,omitempty"`
	DurationMs   int64  `json:"durationMs"`
	ExitReason   string `json:"exitReason,omitempty"`
	ExitCode     *int   `json:"exitCode,omitempty"`
	Error        string `json:"error,omitempty"`
	LogCount     int    `json:"logCount"`
	Running      bool   `json:"running"`
	Stopping     bool   `json:"stopping"`
}

type SessionLogEntry struct {
	SessionID string `json:"sessionId"`
	Sequence  uint64 `json:"sequence"`
	Timestamp string `json:"timestamp"`
	Stream    string `json:"stream"`
	Message   string `json:"message"`
}

type session struct {
	info       SessionInfo
	request    RunRequest
	cmd        *exec.Cmd
	commandCtx context.Context
	cancel     context.CancelFunc
	stdout     io.ReadCloser
	stderr     io.ReadCloser
	stdin      io.WriteCloser
	scriptPath string
	startedAt  time.Time
	logs       []SessionLogEntry
	logBytes   int
	nextLogSeq uint64
	stopping   bool
	finished   bool
	done       chan struct{}
	streamWG   sync.WaitGroup
	pipeClose  sync.Once
}

type Runner struct {
	log                    Logger
	tools                  *toolchain.Resolver
	mu                     sync.Mutex
	sessions               map[string]*session
	sessionEmit            SessionEmitter
	sessionLogEmit         SessionLogEmitter
	sessionLogOrderMu      sync.Mutex
	maxSessionHistory      int
	maxLogsPerSession      int
	maxLogBytesPerSession  int
	maxSessionLogLineBytes int
	findFridaTool          func(string) toolchain.Tool
	commandContext         func(context.Context, string, ...string) *exec.Cmd
}

func (r *Runner) SetSessionEmitter(emit SessionEmitter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessionEmit = emit
}

func (r *Runner) SetSessionLogEmitter(emit SessionLogEmitter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessionLogEmit = emit
}

func NewRunner(log Logger, tools *toolchain.Resolver) *Runner {
	if tools == nil {
		tools = toolchain.NewResolver()
	}
	runner := &Runner{
		log:                    log,
		tools:                  tools,
		sessions:               make(map[string]*session),
		maxSessionHistory:      defaultMaxSessionHistory,
		maxLogsPerSession:      defaultMaxLogsPerSession,
		maxLogBytesPerSession:  defaultMaxLogBytesPerSession,
		maxSessionLogLineBytes: defaultMaxSessionLogLineBytes,
		findFridaTool:          tools.FindFridaTool,
		commandContext:         exec.CommandContext,
	}
	cleanupStaleTempScripts(time.Now(), 7*24*time.Hour)
	return runner
}

func (r *Runner) Status(ctx context.Context) ToolStatus {
	status := ToolStatus{Name: "frida"}
	tool := r.tools.FindFridaTool("frida")
	if !tool.Found {
		status.Error = tool.Error
		return status
	}
	status.Found = true
	status.Path = tool.Path
	status.Source = tool.Source

	cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := processutil.HideWindow(exec.CommandContext(cmdCtx, tool.Path, "--version"))
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

	if ctx == nil {
		ctx = context.Background()
	}
	sessionID := newID()
	cmdCtx, cancel := context.WithCancel(ctx)
	fridaTool := r.findFridaTool("frida")
	if !fridaTool.Found {
		cancel()
		_ = os.Remove(scriptPath)
		return SessionInfo{}, fmt.Errorf("frida not found: %s", fridaTool.Error)
	}
	cmd := processutil.HideWindow(r.commandContext(cmdCtx, fridaTool.Path, args...))

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

	startedAt := time.Now()
	info := SessionInfo{
		ID:           sessionID,
		DeviceSerial: req.DeviceSerial,
		Mode:         req.Mode,
		TargetKind:   req.TargetKind,
		Target:       req.Target,
		ScriptName:   displayScriptName(req.ScriptName),
		StartedAt:    startedAt.Format(time.RFC3339Nano),
		DurationMs:   0,
		Running:      true,
	}

	sess := &session{
		info:       info,
		request:    req,
		cmd:        cmd,
		commandCtx: cmdCtx,
		cancel:     cancel,
		stdout:     stdout,
		stderr:     stderr,
		stdin:      stdin,
		scriptPath: scriptPath,
		startedAt:  startedAt,
		done:       make(chan struct{}),
	}
	sess.streamWG.Add(2)
	r.mu.Lock()
	r.sessions[sessionID] = sess
	r.mu.Unlock()

	r.emitSession(info)
	r.logf("info", "frida", "启动会话 %s: frida %s", sessionID, strings.Join(args, " "))
	go r.stream(sess, "stdout", stdout, "info")
	go r.stream(sess, "stderr", stderr, "error")
	go r.wait(sess)

	return info, nil
}

func (r *Runner) StopSession(sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id is required")
	}

	r.mu.Lock()
	sess, ok := r.sessions[sessionID]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("session %s not found", sessionID)
	}
	if !sess.info.Running {
		r.mu.Unlock()
		// Stop is intentionally idempotent. The UI can have a stale running
		// snapshot while the reaper is finalizing the session.
		return nil
	}
	if sess.stopping {
		r.mu.Unlock()
		return nil
	}
	sess.stopping = true
	sess.info.Stopping = true
	info := sess.info
	r.mu.Unlock()
	r.emitSession(info)

	sess.cancel()
	_ = sess.stdin.Close()
	_ = processutil.Terminate(sess.cmd)
	sess.closePipes()
	r.logf("info", "frida", "正在停止会话 %s", sessionID)
	return nil
}

func (sess *session) closePipes() {
	sess.pipeClose.Do(func() {
		if sess.stdout != nil {
			_ = sess.stdout.Close()
		}
		if sess.stderr != nil {
			_ = sess.stderr.Close()
		}
	})
}

func (r *Runner) StopAll() {
	for _, info := range r.ListSessions() {
		if info.Running {
			_ = r.StopSession(info.ID)
		}
	}
}

// StopAllAndWait stops active sessions and waits until their processes, pipe
// readers, temporary scripts, and child contexts have been cleaned up. The
// caller controls the maximum shutdown delay through ctx.
func (r *Runner) StopAllAndWait(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	type pendingSession struct {
		id         string
		done       <-chan struct{}
		scriptPath string
		stop       bool
	}
	r.mu.Lock()
	pending := make([]pendingSession, 0, len(r.sessions))
	for id, sess := range r.sessions {
		if sess.done == nil || channelClosed(sess.done) {
			continue
		}
		pending = append(pending, pendingSession{
			id:         id,
			done:       sess.done,
			scriptPath: sess.scriptPath,
			stop:       sess.info.Running,
		})
	}
	r.mu.Unlock()
	for _, item := range pending {
		if item.stop {
			_ = r.StopSession(item.id)
		}
	}
	for index, item := range pending {
		select {
		case <-item.done:
		case <-ctx.Done():
			// Cleanup is idempotent. Make a best-effort privacy cleanup for
			// every session that did not finish before the shutdown deadline.
			for _, remaining := range pending[index:] {
				r.removeTempScript(remaining.scriptPath)
			}
			return
		}
	}
}

func channelClosed(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
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

func (r *Runner) wait(sess *session) {
	// StdoutPipe/StderrPipe must be drained before Wait closes their handles.
	// The child closing its handles wakes both scanners, including after Kill.
	sess.streamWG.Wait()
	err := sess.cmd.Wait()
	sessionID := sess.info.ID

	var stdin io.WriteCloser
	var info SessionInfo
	var finished bool
	r.mu.Lock()
	if sess, ok := r.sessions[sessionID]; ok && !sess.finished {
		finishedAt := time.Now()
		sess.finished = true
		sess.info.Running = false
		sess.info.Stopping = false
		sess.info.EndedAt = finishedAt.Format(time.RFC3339Nano)
		sess.info.DurationMs = maxDurationMilliseconds(finishedAt.Sub(sess.startedAt))
		applyExitResult(&sess.info, err, sess.stopping, sess.cmd, sess.commandCtx)
		stdin = sess.stdin
		info = sess.info
		finished = true
	}
	r.mu.Unlock()
	// applyExitResult must observe the command context before it is cancelled;
	// cancelling here then releases the child registration from the app context.
	sess.cancel()
	if stdin != nil {
		_ = stdin.Close()
	}
	r.removeTempScript(sess.scriptPath)
	if finished {
		// done means the process, its context, pipes, and temporary script have
		// all been cleaned up. Shutdown and restart callers rely on this order.
		close(sess.done)
		r.mu.Lock()
		r.pruneHistoryLocked()
		r.mu.Unlock()
	}
	if finished {
		r.emitSession(info)
	}

	if info.ExitReason == "stopped" {
		r.logf("info", "frida", "会话 %s 已停止", sessionID)
		return
	}
	if err != nil {
		r.logf("error", "frida", "会话 %s 已退出: %v", sessionID, err)
		return
	}
	r.logf("info", "frida", "会话 %s 已退出", sessionID)
}

func (r *Runner) emitSession(info SessionInfo) {
	r.mu.Lock()
	emit := r.sessionEmit
	r.mu.Unlock()
	if emit != nil {
		emit(info)
	}
}

func (r *Runner) emitSessionLog(entry SessionLogEntry) {
	r.mu.Lock()
	emit := r.sessionLogEmit
	r.mu.Unlock()
	if emit != nil {
		emit(entry)
	}
}

func (r *Runner) stream(sess *session, pipeName string, reader io.Reader, level string) {
	defer sess.streamWG.Done()
	buffered := bufio.NewReaderSize(reader, 32*1024)
	for {
		line, truncated, err := readBoundedLine(buffered, r.maxSessionLogLineBytes)
		if line != nil {
			message := string(line)
			if truncated {
				message += " … [output truncated]"
			}
			r.recordAndEmitSessionLog(sess.info.ID, pipeName, message)
			r.logf(level, "frida:"+sess.info.ID+":"+pipeName, message)
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				message := "读取输出失败: " + err.Error()
				r.recordAndEmitSessionLog(sess.info.ID, pipeName, message)
				r.logf("error", "frida:"+sess.info.ID, err.Error())
			}
			return
		}
	}
}

// readBoundedLine always drains the full physical line from reader while only
// retaining at most limit bytes. This avoids Scanner's token-limit deadlock
// when an untrusted script writes a very large line to stdout or stderr.
func readBoundedLine(reader *bufio.Reader, limit int) ([]byte, bool, error) {
	if limit <= 0 {
		limit = defaultMaxSessionLogLineBytes
	}
	line := make([]byte, 0, min(limit, 1024))
	truncated := false
	for {
		fragment, prefix, err := reader.ReadLine()
		if len(fragment) > 0 {
			remaining := limit - len(line)
			if remaining > 0 {
				keep := min(remaining, len(fragment))
				line = append(line, fragment[:keep]...)
			}
			if len(fragment) > remaining {
				truncated = true
			}
		}
		if err != nil {
			if len(line) > 0 || truncated {
				return line, truncated, err
			}
			return nil, false, err
		}
		if prefix {
			truncated = truncated || len(line) >= limit
			continue
		}
		// A nil, non-prefixed fragment represents a real blank line.
		return line, truncated, nil
	}
}

func (r *Runner) recordAndEmitSessionLog(sessionID string, stream string, message string) {
	// stdout and stderr are consumed concurrently. Serializing assignment and
	// emission guarantees consumers observe the same sequence as retained logs.
	r.sessionLogOrderMu.Lock()
	defer r.sessionLogOrderMu.Unlock()
	entry, recorded := r.recordSessionLog(sessionID, stream, message)
	if recorded {
		r.emitSessionLog(entry)
	}
}

func (r *Runner) recordSessionLog(sessionID string, stream string, message string) (SessionLogEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, ok := r.sessions[sessionID]
	if !ok {
		return SessionLogEntry{}, false
	}
	sess.nextLogSeq++
	entry := SessionLogEntry{
		SessionID: sessionID,
		Sequence:  sess.nextLogSeq,
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Stream:    stream,
		Message:   message,
	}
	sess.logs = append(sess.logs, entry)
	sess.logBytes += len(entry.Message)
	remove := 0
	for remove < len(sess.logs) {
		overCount := r.maxLogsPerSession > 0 && len(sess.logs)-remove > r.maxLogsPerSession
		overBytes := r.maxLogBytesPerSession > 0 && sess.logBytes > r.maxLogBytesPerSession
		if !overCount && !overBytes {
			break
		}
		sess.logBytes -= len(sess.logs[remove].Message)
		remove++
	}
	if remove > 0 {
		copy(sess.logs, sess.logs[remove:])
		newLength := len(sess.logs) - remove
		// Clear duplicate references left in the backing array. Without this,
		// evicted large messages remain live and defeat the byte budget.
		clear(sess.logs[newLength:])
		sess.logs = sess.logs[:newLength]
	}
	sess.info.LogCount = len(sess.logs)
	return entry, true
}

// GetSessionLogs returns a stable copy of the retained stdout/stderr lines for
// one active or completed session. The copy keeps callers from racing with the
// scanner goroutines that append new output.
func (r *Runner) GetSessionLogs(sessionID string) ([]SessionLogEntry, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	sess, ok := r.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}
	logs := make([]SessionLogEntry, len(sess.logs))
	copy(logs, sess.logs)
	return logs, nil
}

// RestartSession starts a new session from the original immutable request. If
// the old session is still active, it is stopped first so reinjection cannot
// accidentally leave two Frida CLI processes attached to the same target.
func (r *Runner) RestartSession(ctx context.Context, sessionID string) (SessionInfo, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return SessionInfo{}, errors.New("session id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	r.mu.Lock()
	sess, ok := r.sessions[sessionID]
	if !ok {
		r.mu.Unlock()
		return SessionInfo{}, fmt.Errorf("session %s not found", sessionID)
	}
	req := sess.request
	running := sess.info.Running
	done := sess.done
	r.mu.Unlock()

	if running {
		if err := r.StopSession(sessionID); err != nil {
			return SessionInfo{}, err
		}
		select {
		case <-done:
		case <-ctx.Done():
			return SessionInfo{}, fmt.Errorf("waiting for session %s to stop: %w", sessionID, ctx.Err())
		case <-time.After(5 * time.Second):
			return SessionInfo{}, fmt.Errorf("timed out waiting for session %s to stop", sessionID)
		}
	}

	return r.RunScript(ctx, req)
}

// ClearSessionHistory drops completed session metadata and logs while keeping
// active processes intact. It returns the number of removed sessions.
func (r *Runner) ClearSessionHistory() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	removed := 0
	for id, sess := range r.sessions {
		if sess.info.Running {
			continue
		}
		delete(r.sessions, id)
		removed++
	}
	return removed
}

func applyExitResult(info *SessionInfo, waitErr error, stopping bool, cmd *exec.Cmd, cmdCtx context.Context) {
	if cmd != nil && cmd.ProcessState != nil {
		exitCode := cmd.ProcessState.ExitCode()
		info.ExitCode = &exitCode
	}

	if stopping {
		info.ExitReason = "stopped"
		info.Error = ""
		return
	}
	if cmdCtx != nil && cmdCtx.Err() != nil {
		info.ExitReason = "cancelled"
		info.Error = cmdCtx.Err().Error()
		return
	}
	if waitErr == nil {
		info.ExitReason = "completed"
		if info.ExitCode == nil {
			exitCode := 0
			info.ExitCode = &exitCode
		}
		return
	}
	info.ExitReason = "failed"
	info.Error = waitErr.Error()
}

func maxDurationMilliseconds(duration time.Duration) int64 {
	if duration <= 0 {
		return 0
	}
	milliseconds := duration.Milliseconds()
	if milliseconds == 0 {
		return 1
	}
	return milliseconds
}

func (r *Runner) pruneHistoryLocked() {
	limit := r.maxSessionHistory
	type historyItem struct {
		id        string
		startedAt time.Time
	}
	completed := make([]historyItem, 0, len(r.sessions))
	for id, sess := range r.sessions {
		if !sess.info.Running {
			completed = append(completed, historyItem{id: id, startedAt: sess.startedAt})
		}
	}
	if len(completed) <= limit && limit >= 0 {
		return
	}
	sort.Slice(completed, func(i, j int) bool {
		return completed[i].startedAt.Before(completed[j].startedAt)
	})
	removeCount := len(completed) - limit
	if limit < 0 {
		removeCount = 0
	}
	for i := 0; i < removeCount; i++ {
		delete(r.sessions, completed[i].id)
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
	dir := filepath.Join(os.TempDir(), "frida-gui-helper", fmt.Sprintf("process-%d", os.Getpid()))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "hook-"+newID()+".js")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (r *Runner) removeTempScript(scriptPath string) {
	scriptPath = strings.TrimSpace(scriptPath)
	if scriptPath == "" {
		return
	}
	if err := os.Remove(scriptPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		r.logf("warn", "frida", "删除会话临时脚本失败: %v", err)
		return
	}

	// Remove only the empty process directory created by writeTempScript.
	dir := filepath.Clean(filepath.Dir(scriptPath))
	base := filepath.Clean(filepath.Join(os.TempDir(), "frida-gui-helper"))
	if filepath.Dir(dir) == base && strings.HasPrefix(filepath.Base(dir), "process-") {
		_ = os.Remove(dir)
	}
}

func cleanupStaleTempScripts(now time.Time, maxAge time.Duration) {
	base := filepath.Join(os.TempDir(), "frida-gui-helper")
	cleanupStaleTempScriptsAt(base, now, maxAge)
}

func cleanupStaleTempScriptsAt(base string, now time.Time, maxAge time.Duration) {
	cutoff := now.Add(-maxAge)
	entries, err := os.ReadDir(base)
	if err != nil {
		return
	}
	removeOldHooks := func(dir string) {
		files, readErr := os.ReadDir(dir)
		if readErr != nil {
			return
		}
		for _, file := range files {
			if file.IsDir() || !strings.HasPrefix(file.Name(), "hook-") || !strings.HasSuffix(file.Name(), ".js") {
				continue
			}
			info, infoErr := file.Info()
			if infoErr == nil && info.ModTime().Before(cutoff) {
				_ = os.Remove(filepath.Join(dir, file.Name()))
			}
		}
	}
	removeOldHooks(base) // Clean files created by v1.0.8 and older builds.
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "process-") {
			continue
		}
		dir := filepath.Join(base, entry.Name())
		removeOldHooks(dir)
		_ = os.Remove(dir) // Succeeds only when the process directory is empty.
	}
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
