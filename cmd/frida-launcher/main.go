package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"frida-gui-helper/internal/processutil"
)

func main() {
	executable, err := os.Executable()
	if err != nil {
		fatal(err)
	}
	root := filepath.Dir(executable)
	pythonName := "python"
	if runtime.GOOS == "windows" {
		pythonName += ".exe"
	}
	pythonPath := filepath.Join(root, "python", pythonName)
	launcherPath := filepath.Join(root, "launcher.py")
	toolName := strings.TrimSuffix(strings.ToLower(filepath.Base(executable)), ".exe")

	args := make([]string, 0, len(os.Args)+1)
	args = append(args, launcherPath, toolName)
	args = append(args, os.Args[1:]...)
	cmd := processutil.HideWindow(exec.Command(pythonPath, args...))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = isolatedPythonEnvironment(os.Environ())
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fatal(fmt.Errorf("启动内置 %s 失败: %w", toolName, err))
	}
}

func isolatedPythonEnvironment(current []string) []string {
	env := make([]string, 0, len(current)+3)
	for _, item := range current {
		key := strings.ToUpper(strings.TrimSpace(strings.SplitN(item, "=", 2)[0]))
		if key == "PYTHONHOME" || key == "PYTHONPATH" || key == "PYTHONUSERBASE" {
			continue
		}
		env = append(env, item)
	}
	return append(env, "PYTHONNOUSERSITE=1", "PYTHONUTF8=1", "PYTHONDONTWRITEBYTECODE=1")
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
