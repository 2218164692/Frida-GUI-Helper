//go:build windows

package processutil

import (
	"os/exec"
	"strconv"
	"syscall"
)

const createNoWindow = 0x08000000

func HideWindow(cmd *exec.Cmd) *exec.Cmd {
	if cmd == nil {
		return nil
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
	return cmd
}

// Terminate stops the command and all descendants that inherited its handles.
// Frida's Windows CLI can create helper processes; killing only the parent can
// leave stdout/stderr pipes open forever and make a session appear stuck.
func Terminate(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	taskkill := HideWindow(exec.Command("taskkill.exe", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F"))
	if err := taskkill.Run(); err == nil {
		return nil
	}
	// The process may have exited between lookup and taskkill. Fall back to a
	// direct kill so callers still get the same best-effort behavior.
	return cmd.Process.Kill()
}
