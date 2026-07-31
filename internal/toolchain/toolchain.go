package toolchain

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type Tool struct {
	Name   string
	Path   string
	Found  bool
	Source string
	Error  string
}

type Resolver struct {
	mu            sync.RWMutex
	baseDirs      []string
	fridaBaseDirs []string
}

// AddBaseDir registers a managed runtime directory with higher priority than PATH.
// It is used by the runtime installer after downloading a versioned Frida CLI.
func (r *Resolver) AddBaseDir(path string) {
	if r == nil || strings.TrimSpace(path) == "" {
		return
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return
	}
	clean := filepath.Clean(abs)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.baseDirs = prependUnique(r.baseDirs, clean)
}

func prependUnique(values []string, value string) []string {
	for _, existing := range values {
		if strings.EqualFold(existing, value) {
			return values
		}
	}
	return append([]string{value}, values...)
}

func NewResolver() *Resolver {
	seen := map[string]bool{}
	var dirs []string
	var fridaDirs []string
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
		// Frida is deliberately resolved only beside the application. Parent
		// directories and the process working directory remain useful for
		// generic bundled tools, but must never influence the Frida version.
		fridaDirs = append(fridaDirs, filepath.Clean(exeDir))
		add(filepath.Dir(exeDir))
		add(filepath.Dir(filepath.Dir(exeDir)))
	}
	if cwd, err := os.Getwd(); err == nil {
		add(cwd)
	}

	return &Resolver{baseDirs: dirs, fridaBaseDirs: fridaDirs}
}

func (r *Resolver) FindExecutable(name string) Tool {
	tool := r.FindBundledExecutable(name)
	if tool.Found {
		return tool
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

// FindBundledExecutable resolves an executable only from application-managed
// directories. It never falls back to PATH. Frida callers must use this method
// (or FindFridaTool) so a user's global Python/Frida installation cannot change
// the runtime selected by the application.
func (r *Resolver) FindBundledExecutable(name string) Tool {
	tool := Tool{Name: name}
	for _, base := range r.BaseDirs() {
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
	tool.Error = "未在应用内置目录中找到 " + name
	return tool
}

// FindFridaTool is the strict resolver for Frida CLI commands. Keeping a
// dedicated entry point makes it difficult to accidentally reintroduce PATH
// fallback at a call site.
func (r *Resolver) FindFridaTool(name string) Tool {
	tool := Tool{Name: name}
	for _, base := range r.FridaBaseDirs() {
		for _, rel := range fridaExecutableCandidates(name) {
			path := filepath.Join(base, rel)
			if isExecutableFile(path) {
				tool.Found = true
				tool.Path = path
				tool.Source = "bundled"
				return tool
			}
		}
	}
	tool.Error = "未找到安装包内置 " + name + "；请重新安装或在设置中更新 Frida 组件"
	return tool
}

// FindBundledPython returns the relocatable Python shipped below frida/python.
// It deliberately ignores Python installations registered in PATH.
func (r *Resolver) FindBundledPython() Tool {
	tool := Tool{Name: "python"}
	for _, base := range r.FridaBaseDirs() {
		for _, rel := range []string{
			filepath.FromSlash("frida/python/python.exe"),
			filepath.FromSlash("frida/host/python.exe"),
			filepath.FromSlash("tools/frida/python/python.exe"),
		} {
			path := filepath.Join(base, rel)
			if isExecutableFile(path) {
				tool.Found = true
				tool.Path = path
				tool.Source = "bundled"
				return tool
			}
		}
	}
	tool.Error = "未找到安装包内置 Python；请重新安装或在设置中更新 Frida 组件"
	return tool
}

// FridaRoot returns the managed Frida component directory. Existing manifests
// take priority. If the component is missing, the path beside the executable is
// returned so diagnostics can point at the expected installation location.
func (r *Resolver) FridaRoot() string {
	bases := r.FridaBaseDirs()
	for _, base := range bases {
		root := filepath.Join(base, "frida")
		if isRegularFile(filepath.Join(root, "manifest.json")) {
			return root
		}
	}
	for _, base := range bases {
		root := filepath.Join(base, "frida")
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			return root
		}
	}
	if len(bases) > 0 {
		return filepath.Join(bases[0], "frida")
	}
	return "frida"
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
	add("frida/server/android-" + arch + "/frida-server")
	add("frida/servers/android-" + arch + "/frida-server")
	add("frida/server/" + arch + "/frida-server")
	if version != "" {
		add("frida/server/" + version + "/android-" + arch + "/frida-server")
		add("frida/servers/" + version + "/android-" + arch + "/frida-server")
	}

	for _, base := range r.FridaBaseDirs() {
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

	tool.Error = "未找到安装包内置 frida-server: frida/server/android-" + arch + "/frida-server"
	return tool
}

func (r *Resolver) BaseDirs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	copied := make([]string, len(r.baseDirs))
	copy(copied, r.baseDirs)
	return copied
}

// FridaBaseDirs returns only explicitly managed roots. For a normal desktop
// launch this contains the executable directory and excludes PATH, parent
// directories, and the current working directory.
func (r *Resolver) FridaBaseDirs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	values := r.fridaBaseDirs
	// Preserve compatibility with resolvers assembled directly in tests while
	// still limiting strict Frida resolution to one explicit root.
	if len(values) == 0 && len(r.baseDirs) > 0 {
		values = r.baseDirs[:1]
	}
	copied := make([]string, len(values))
	copy(copied, values)
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
			filepath.Join("frida", n),
			filepath.Join("frida", "bin", n),
			filepath.Join("frida", "Scripts", n),
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

func fridaExecutableCandidates(name string) []string {
	names := []string{name}
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		names = append(names, name+".exe")
	}
	var rels []string
	for _, candidate := range names {
		rels = append(rels,
			filepath.Join("frida", candidate),
			filepath.Join("frida", "bin", candidate),
			filepath.Join("frida", "Scripts", candidate),
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
