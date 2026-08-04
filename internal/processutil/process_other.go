//go:build !windows

package processutil

import "os/exec"

func HideWindow(cmd *exec.Cmd) *exec.Cmd {
	return cmd
}

func Terminate(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
