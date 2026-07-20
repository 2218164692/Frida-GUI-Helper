//go:build windows

package processutil

import (
	"os/exec"
	"testing"
)

func TestHideWindow(t *testing.T) {
	cmd := HideWindow(exec.Command("cmd.exe", "/c", "exit", "0"))
	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr was not configured")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("HideWindow is false")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatalf("CreationFlags = %#x", cmd.SysProcAttr.CreationFlags)
	}
}
