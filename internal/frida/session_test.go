package frida

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"frida-gui-helper/internal/toolchain"
)

func TestSessionHistoryCapturesOutputAndExitDetails(t *testing.T) {
	runner := newHelperRunner("output")
	logEvents := make(chan SessionLogEntry, 4)
	runner.SetSessionLogEmitter(func(entry SessionLogEntry) {
		logEvents <- entry
	})

	started, err := runner.RunScript(context.Background(), RunRequest{
		DeviceSerial: "device-1",
		Mode:         "attach",
		TargetKind:   "package",
		Target:       "com.example.app",
		ScriptName:   "example.js",
		ScriptSource: "console.log('test');",
	})
	if err != nil {
		t.Fatal(err)
	}

	finished := waitForFinishedSession(t, runner, started.ID)
	if finished.Running {
		t.Fatalf("session is still marked running: %+v", finished)
	}
	if finished.EndedAt == "" || finished.DurationMs < 1 {
		t.Fatalf("missing timing details: %+v", finished)
	}
	if finished.ExitReason != "completed" || finished.ExitCode == nil || *finished.ExitCode != 0 {
		t.Fatalf("unexpected exit details: %+v", finished)
	}

	logs, err := runner.GetSessionLogs(started.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 || finished.LogCount != 2 {
		t.Fatalf("logs = %+v, session = %+v", logs, finished)
	}
	streams := map[string]string{}
	for index, entry := range logs {
		streams[entry.Stream] = entry.Message
		if entry.SessionID != started.ID || entry.Sequence != uint64(index+1) || entry.Timestamp == "" {
			t.Fatalf("invalid log entry: %+v", entry)
		}
	}
	if streams["stdout"] != "helper stdout" || streams["stderr"] != "helper stderr" {
		t.Fatalf("unexpected stream output: %+v", streams)
	}

	for i := 0; i < 2; i++ {
		select {
		case event := <-logEvents:
			if event.SessionID != started.ID {
				t.Fatalf("event belongs to another session: %+v", event)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for session log event")
		}
	}
}

func TestRestartSessionUsesOriginalRequest(t *testing.T) {
	runner := newHelperRunner("output")
	first, err := runner.RunScript(context.Background(), RunRequest{
		Mode:         "spawn",
		TargetKind:   "package",
		Target:       "com.example.restart",
		ScriptName:   "restart.js",
		ScriptSource: "console.log('restart');",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForFinishedSession(t, runner, first.ID)

	second, err := runner.RestartSession(context.Background(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID || second.Target != first.Target || second.Mode != first.Mode || second.ScriptName != first.ScriptName {
		t.Fatalf("restart did not preserve request: first=%+v second=%+v", first, second)
	}
	waitForFinishedSession(t, runner, second.ID)

	if got := len(runner.ListSessions()); got != 2 {
		t.Fatalf("completed session history length = %d, want 2", got)
	}
}

func TestStopSessionKeepsStoppedHistory(t *testing.T) {
	runner := newHelperRunner("block")
	started, err := runner.RunScript(context.Background(), RunRequest{
		Mode:         "attach",
		TargetKind:   "pid",
		Target:       "123",
		ScriptSource: "console.log('block');",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.StopSession(started.ID); err != nil {
		t.Fatal(err)
	}
	// A second click while the process is still being reaped must be a
	// no-op, rather than issuing another kill/logging another stop request.
	if err := runner.StopSession(started.ID); err != nil {
		t.Fatalf("repeated stop should be idempotent: %v", err)
	}

	finished := waitForFinishedSession(t, runner, started.ID)
	if finished.ExitReason != "stopped" || finished.Running || finished.Stopping || finished.EndedAt == "" {
		t.Fatalf("unexpected stopped session: %+v", finished)
	}
	if _, err := runner.GetSessionLogs(started.ID); err != nil {
		t.Fatalf("stopped session was not retained: %v", err)
	}
}

func TestSessionLogLimitRetainsNewestLines(t *testing.T) {
	runner := NewRunner(nil, nil)
	runner.maxLogsPerSession = 2
	runner.sessions["session-1"] = &session{
		info: SessionInfo{ID: "session-1", Running: true},
		done: make(chan struct{}),
	}

	for i := 1; i <= 3; i++ {
		if _, ok := runner.recordSessionLog("session-1", "stdout", strconv.Itoa(i)); !ok {
			t.Fatal("failed to record log")
		}
	}
	logs, err := runner.GetSessionLogs("session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 || logs[0].Message != "2" || logs[1].Message != "3" || logs[0].Sequence != 2 || logs[1].Sequence != 3 {
		t.Fatalf("unexpected retained logs: %+v", logs)
	}
}

func TestReadBoundedLineTruncatesAndDrainsPhysicalLine(t *testing.T) {
	reader := bufio.NewReaderSize(strings.NewReader(strings.Repeat("x", 2*1024*1024)+"\nnext\n"), 64)
	line, truncated, err := readBoundedLine(reader, 128)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated || len(line) != 128 || string(line) != strings.Repeat("x", 128) {
		t.Fatalf("first line length=%d truncated=%v", len(line), truncated)
	}

	line, truncated, err = readBoundedLine(reader, 128)
	if err != nil {
		t.Fatal(err)
	}
	if truncated || string(line) != "next" {
		t.Fatalf("second line=%q truncated=%v; oversized line was not fully drained", line, truncated)
	}
}

func TestOversizedSessionOutputIsTruncatedWithoutBlockingExit(t *testing.T) {
	runner := newHelperRunner("long-line")
	runner.maxSessionLogLineBytes = 128
	started, err := runner.RunScript(context.Background(), RunRequest{
		Mode:         "attach",
		TargetKind:   "pid",
		Target:       "123",
		ScriptSource: "console.log('long');",
	})
	if err != nil {
		t.Fatal(err)
	}
	finished := waitForFinishedSession(t, runner, started.ID)
	if finished.ExitReason != "completed" {
		t.Fatalf("oversized output prevented normal exit: %+v", finished)
	}
	logs, err := runner.GetSessionLogs(started.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 {
		t.Fatalf("logs=%+v, want truncated line and following line", logs)
	}
	if !strings.HasPrefix(logs[0].Message, strings.Repeat("x", 128)) || !strings.HasSuffix(logs[0].Message, "[output truncated]") {
		t.Fatalf("oversized line was not visibly truncated: %q", logs[0].Message)
	}
	if logs[1].Message != "after-long-line" {
		t.Fatalf("line after oversized output was lost: %+v", logs)
	}
}

func TestSessionLogByteBudgetEvictsAndReleasesOldMessages(t *testing.T) {
	runner := NewRunner(nil, nil)
	runner.maxLogsPerSession = 10
	runner.maxLogBytesPerSession = 4
	sess := &session{
		info: SessionInfo{ID: "session-budget", Running: true},
		logs: make([]SessionLogEntry, 0, 4),
		done: make(chan struct{}),
	}
	runner.sessions[sess.info.ID] = sess

	for _, message := range []string{"aaaa", "bbbb"} {
		if _, ok := runner.recordSessionLog(sess.info.ID, "stdout", message); !ok {
			t.Fatal("failed to record log")
		}
	}
	logs, err := runner.GetSessionLogs(sess.info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Message != "bbbb" || sess.logBytes != 4 {
		t.Fatalf("logs=%+v bytes=%d, want newest message within budget", logs, sess.logBytes)
	}
	backing := sess.logs[:cap(sess.logs)]
	for index := len(sess.logs); index < len(backing); index++ {
		if backing[index].Message != "" {
			t.Fatalf("evicted message is still retained in backing slot %d: %q", index, backing[index].Message)
		}
	}
}

func TestStopAllAndWaitRemovesTemporaryScriptBeforeReturning(t *testing.T) {
	runner := newHelperRunner("block")
	started, err := runner.RunScript(context.Background(), RunRequest{
		Mode:         "attach",
		TargetKind:   "pid",
		Target:       "123",
		ScriptSource: "console.log('private source');",
	})
	if err != nil {
		t.Fatal(err)
	}
	runner.mu.Lock()
	scriptPath := runner.sessions[started.ID].scriptPath
	runner.mu.Unlock()
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("temporary script was not created: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	runner.StopAllAndWait(ctx)
	if err := ctx.Err(); err != nil {
		t.Fatalf("StopAllAndWait exceeded its deadline: %v", err)
	}
	if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
		t.Fatalf("temporary script still exists after StopAllAndWait: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(scriptPath)); !os.IsNotExist(err) {
		t.Fatalf("empty per-process temp directory still exists after StopAllAndWait: %v", err)
	}
	finished := waitForFinishedSession(t, runner, started.ID)
	if finished.ExitReason != "stopped" {
		t.Fatalf("unexpected stopped session: %+v", finished)
	}
}

func TestCleanupStaleTempScriptsRemovesOnlyExpiredHooks(t *testing.T) {
	base := t.TempDir()
	processDir := filepath.Join(base, "process-123")
	if err := os.MkdirAll(processDir, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	oldHook := filepath.Join(processDir, "hook-old.js")
	freshHook := filepath.Join(base, "hook-fresh.js")
	unrelated := filepath.Join(processDir, "keep.txt")
	for path, contents := range map[string]string{
		oldHook:   "old secret",
		freshHook: "active secret",
		unrelated: "not managed by the cleanup",
	} {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	oldTime := now.Add(-2 * time.Hour)
	if err := os.Chtimes(oldHook, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	cleanupStaleTempScriptsAt(base, now, time.Hour)
	if _, err := os.Stat(oldHook); !os.IsNotExist(err) {
		t.Fatalf("expired hook was not removed: %v", err)
	}
	for _, path := range []string{freshHook, unrelated} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("cleanup removed unrelated or fresh file %s: %v", path, err)
		}
	}
}

func TestClearSessionHistoryPreservesRunningSessions(t *testing.T) {
	runner := NewRunner(nil, nil)
	runner.sessions["running"] = &session{info: SessionInfo{ID: "running", Running: true}}
	runner.sessions["finished"] = &session{info: SessionInfo{ID: "finished", Running: false}}

	if removed := runner.ClearSessionHistory(); removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	listed := runner.ListSessions()
	if len(listed) != 1 || listed[0].ID != "running" {
		t.Fatalf("unexpected remaining sessions: %+v", listed)
	}
}

func TestCompletedSessionHistoryIsBounded(t *testing.T) {
	runner := NewRunner(nil, nil)
	runner.maxSessionHistory = 2
	for i := 1; i <= 3; i++ {
		id := strconv.Itoa(i)
		runner.sessions[id] = &session{
			info:      SessionInfo{ID: id, Running: false},
			startedAt: time.Unix(int64(i), 0),
		}
	}

	runner.mu.Lock()
	runner.pruneHistoryLocked()
	runner.mu.Unlock()

	if _, ok := runner.sessions["1"]; ok {
		t.Fatal("oldest completed session was not pruned")
	}
	if len(runner.sessions) != 2 {
		t.Fatalf("history length = %d, want 2", len(runner.sessions))
	}
}

func TestFridaSessionHelperProcess(t *testing.T) {
	if os.Getenv("FRIDA_GUI_HELPER_TEST_PROCESS") != "1" {
		return
	}
	switch os.Getenv("FRIDA_GUI_HELPER_TEST_BEHAVIOR") {
	case "output":
		fmt.Fprintln(os.Stdout, "helper stdout")
		fmt.Fprintln(os.Stderr, "helper stderr")
		os.Exit(0)
	case "block":
		for {
			time.Sleep(time.Hour)
		}
	case "long-line":
		fmt.Fprintln(os.Stdout, strings.Repeat("x", 2*1024*1024))
		fmt.Fprintln(os.Stdout, "after-long-line")
		os.Exit(0)
	default:
		os.Exit(2)
	}
}

func newHelperRunner(behavior string) *Runner {
	runner := NewRunner(nil, nil)
	runner.findFridaTool = func(string) toolchain.Tool {
		return toolchain.Tool{Name: "frida", Path: os.Args[0], Found: true, Source: "test"}
	}
	runner.commandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestFridaSessionHelperProcess")
		cmd.Env = append(os.Environ(),
			"FRIDA_GUI_HELPER_TEST_PROCESS=1",
			"FRIDA_GUI_HELPER_TEST_BEHAVIOR="+behavior,
		)
		return cmd
	}
	return runner
}

func waitForFinishedSession(t *testing.T, runner *Runner, sessionID string) SessionInfo {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, info := range runner.ListSessions() {
			if info.ID == sessionID && !info.Running {
				return info
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for session %s to finish", sessionID)
	return SessionInfo{}
}
