package toolchain

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Tool struct {
	Name   string
	Path   string
	Found  bool
	Source string
	Error  string
}

type Resolver struct {
	baseDirs []string
}

func NewResolver() *Resolver {
	seen := map[string]bool{}
	var dirs []string
	add := func(path string) {
		if path == "" {
			return
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return
		}
		clean := filepath.Clean(abs)
		key := strings.ToLower(clean)
		if seen[key] {
			return
		}
		seen[key] = true
		dirs = append(dirs, clean)
	}

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		add(exeDir)
		add(filepath.Dir(exeDir))
		add(filepath.Dir(filepath.Dir(exeDir)))
	}
	if cwd, err := os.Getwd(); err == nil {
		add(cwd)
	}

	return &Resolver{baseDirs: dirs}
}

func (r *Resolver) FindExecutable(name string) Tool {
	tool := Tool{Name: name}
	for _, base := range r.baseDirs {
		for _, rel := range executableCandidates(name) {
			path := filepath.Join(base, rel)
			if isExecutableFile(path) {
				tool.Found = true
				tool.Path = path
				tool.Source = "bundled"
				return tool
			}
		}
	}

	path, err := exec.LookPath(name)
	if err != nil && runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		path, err = exec.LookPath(name + ".exe")
	}
	if err != nil {
		tool.Error = err.Error()
		return tool
	}
	tool.Found = true
	tool.Path = path
	tool.Source = "PATH"
	return tool
}

func (r *Resolver) FindFridaServer(version string, abi string) Tool {
	arch := serverArch(abi)
	tool := Tool{Name: "frida-server"}
	if arch == "" {
		tool.Error = "unsupported Android ABI: " + abi
		return tool
	}

	var rels []string
	add := func(rel string) {
		if rel != "" {
			rels = append(rels, filepath.FromSlash(rel))
		}
	}
	add("tools/frida-server/" + arch + "/frida-server")
	add("tools/frida-server/android-" + arch + "/frida-server")
	add("tools/frida-server/frida-server-android-" + arch)
	if version != "" {
		add("tools/frida-server/" + version + "/" + arch + "/frida-server")
		add("tools/frida-server/" + version + "/android-" + arch + "/frida-server")
		add("tools/frida-server/" + version + "/frida-server-android-" + arch)
		add("tools/frida-server/frida-server-" + version + "-android-" + arch)
	}
	add("tools/frida-server/frida-server")
	add("frida-server")

	for _, base := range r.baseDirs {
		for _, rel := range rels {
			path := filepath.Join(base, rel)
			if isRegularFile(path) {
				tool.Found = true
				tool.Path = path
				tool.Source = "bundled"
				return tool
			}
		}
	}

	tool.Error = "未找到内置 frida-server，请放到 tools/frida-server/android-" + arch + "/frida-server"
	return tool
}

func (r *Resolver) BaseDirs() []string {
	copied := make([]string, len(r.baseDirs))
	copy(copied, r.baseDirs)
	return copied
}

func executableCandidates(name string) []string {
	names := []string{name}
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		names = append(names, name+".exe")
	}

	var rels []string
	for _, n := range names {
		rels = append(rels,
			filepath.Join("tools", "bin", n),
			filepath.Join("tools", "frida", n),
			filepath.Join("tools", "adb", n),
			filepath.Join("tools", "platform-tools", n),
			filepath.Join("platform-tools", n),
			filepath.Join("bin", n),
			n,
		)
	}
	return rels
}

func serverArch(abi string) string {
	abi = strings.ToLower(strings.TrimSpace(abi))
	switch {
	case strings.Contains(abi, "arm64"):
		return "arm64"
	case strings.Contains(abi, "armeabi") || strings.Contains(abi, "arm"):
		return "arm"
	case strings.Contains(abi, "x86_64"):
		return "x86_64"
	case strings.Contains(abi, "x86"):
		return "x86"
	default:
		return ""
	}
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
