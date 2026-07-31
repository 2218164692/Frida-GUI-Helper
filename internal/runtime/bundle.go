package runtime

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	bundleManifestName  = "manifest.json"
	bundleSchemaVersion = 1
)

var androidServerArchitectures = []string{"arm64", "arm", "x86_64", "x86"}

var bundleLauncherFiles = []string{
	"frida.exe",
	"frida-ps.exe",
	"frida-ls-devices.exe",
	"launcher.py",
}

const bundleSwapJournalMagic = "Frida-GUI-Helper-bundle-swap-v1"

type bundleSwapJournal struct {
	Magic      string `json:"magic"`
	BackupName string `json:"backupName"`
}

type BundleFile struct {
	Path        string `json:"path"`
	SHA256      string `json:"sha256"`
	AssetSHA256 string `json:"assetSha256,omitempty"`
	SourceURL   string `json:"sourceUrl,omitempty"`
}

// BundleManifest is the single source of truth for the Frida version used by
// both the desktop CLI and every Android server shipped with the installer.
type BundleManifest struct {
	SchemaVersion     int                   `json:"schemaVersion"`
	Version           string                `json:"version"`
	FridaToolsVersion string                `json:"fridaToolsVersion"`
	PythonVersion     string                `json:"pythonVersion"`
	ReleaseURL        string                `json:"releaseUrl"`
	BuiltAt           string                `json:"builtAt"`
	Servers           map[string]BundleFile `json:"servers"`
}

func (m *Manager) ReadBundleManifest() (BundleManifest, error) {
	path := filepath.Join(m.bundleRoot, bundleManifestName)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return BundleManifest{}, fmt.Errorf("Frida 组件清单不存在: %s", path)
		}
		return BundleManifest{}, fmt.Errorf("读取 Frida 组件清单失败: %w", err)
	}
	if len(data) > 1024*1024 {
		return BundleManifest{}, errors.New("Frida 组件清单异常：文件过大")
	}
	var manifest BundleManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return BundleManifest{}, fmt.Errorf("解析 Frida 组件清单失败: %w", err)
	}
	manifest.Version = normalizeVersion(manifest.Version)
	if manifest.SchemaVersion != bundleSchemaVersion || manifest.Version == "" || manifest.Version == "latest" {
		return BundleManifest{}, errors.New("Frida 组件清单版本无效")
	}
	return manifest, nil
}

// BundledStatus inspects only files below BundleRoot. It must not use the
// generic tool resolver because that resolver is allowed to find ADB in PATH.
func (m *Manager) BundledStatus(ctx context.Context, abi string) Status {
	m.bundleMu.RLock()
	defer m.bundleMu.RUnlock()
	return m.bundledStatus(ctx, abi)
}

func (m *Manager) bundledStatus(ctx context.Context, abi string) Status {
	status := Status{CacheDir: m.bundleRoot}
	manifest, manifestErr := m.ReadBundleManifest()
	if manifestErr == nil {
		status.LatestVersion = manifest.Version
	}

	hostPath := bundleExecutablePath(m.bundleRoot, "frida")
	status.Host = ToolStatus{Name: "frida", Path: hostPath, Source: "bundled"}
	if bundleFileExists(hostPath) {
		status.Host.Found = true
		status.Host.Version = normalizeVersion(commandVersion(ctx, hostPath))
		if manifestErr == nil && status.Host.Version != manifest.Version {
			status.Host.Found = false
			status.Host.Error = fmt.Sprintf("内置 Frida CLI 版本 %s 与组件清单 %s 不一致", status.Host.Version, manifest.Version)
		}
	} else {
		status.Host.Error = "安装包内置 frida.exe 缺失，请重新安装或更新 Frida 组件"
	}
	if manifestErr != nil && status.Host.Error == "" {
		status.Host.Error = manifestErr.Error()
	}

	pythonPath := filepath.Join(m.bundleRoot, "python", executableName("python"))
	status.Python = ToolStatus{Name: "python", Path: pythonPath, Source: "bundled"}
	if bundleFileExists(pythonPath) {
		status.Python.Found = true
		output, err := runOutput(ctx, pythonPath, "--version")
		if err == nil {
			status.Python.Version = firstLine(output)
		} else {
			status.Python.Error = err.Error()
		}
	} else {
		status.Python.Error = "安装包内置 Python 缺失"
	}

	status.Server = ToolStatus{Name: "frida-server", Source: "bundled", Version: status.LatestVersion}
	if strings.TrimSpace(abi) != "" {
		arch, err := normalizeAndroidArch(abi)
		if err != nil {
			status.Server.Error = err.Error()
			return status
		}
		status.Server.Path = bundleServerPath(m.bundleRoot, arch)
		entry, declared := manifest.Servers[arch]
		if manifestErr != nil {
			status.Server.Error = manifestErr.Error()
		} else if !declared {
			status.Server.Error = "组件清单未包含 Android " + arch + " frida-server"
		} else if err := verifyBundleFile(m.bundleRoot, entry); err != nil {
			status.Server.Error = err.Error()
		} else {
			status.Server.Found = true
		}
		return status
	}

	status.Server.Path = filepath.Join(m.bundleRoot, "server")
	if manifestErr != nil {
		status.Server.Error = manifestErr.Error()
		return status
	}
	for _, arch := range androidServerArchitectures {
		entry, ok := manifest.Servers[arch]
		if !ok {
			status.Server.Error = "组件清单缺少 Android " + arch + " frida-server"
			return status
		}
		rel := filepath.Clean(filepath.FromSlash(entry.Path))
		if rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			status.Server.Error = "Frida 组件清单包含不安全路径"
			return status
		}
		if !bundleFileExists(filepath.Join(m.bundleRoot, rel)) {
			status.Server.Error = "Frida 组件文件缺失: " + filepath.Join(m.bundleRoot, rel)
			return status
		}
	}
	status.Server.Found = true
	return status
}

// PrepareBundled validates the installed component and selects the server for
// the connected device. It performs no network requests or system discovery.
func (m *Manager) PrepareBundled(ctx context.Context, abi string) (Status, error) {
	m.emitProgress(Progress{Phase: "runtime", Stage: "checking", Message: "正在校验安装包内置 Frida 组件"})
	status := m.BundledStatus(ctx, abi)
	var problems []error
	if !status.Host.Found {
		problems = append(problems, errors.New(status.Host.Error))
	}
	if !status.Python.Found {
		problems = append(problems, errors.New(status.Python.Error))
	}
	if !status.Server.Found {
		problems = append(problems, errors.New(status.Server.Error))
	}
	if len(problems) > 0 {
		err := errors.Join(problems...)
		m.emitFailure("runtime", status.LatestVersion, "", err.Error())
		return status, err
	}
	m.emitProgress(Progress{
		Phase:   "runtime",
		Stage:   "complete",
		Version: status.LatestVersion,
		Percent: 100,
		Message: fmt.Sprintf("内置 Frida %s 已就绪", status.LatestVersion),
		Done:    true,
	})
	return status, nil
}

// InstallBundle downloads one official Frida release, installs its matching
// Python package, and prepares servers for every Android ABI. The manifest is
// written last so an interrupted update cannot be mistaken for a complete one.
func (m *Manager) InstallBundle(ctx context.Context, version string) (BundleManifest, error) {
	m.bundleMu.Lock()
	defer m.bundleMu.Unlock()
	if runtime.GOOS != "windows" {
		return BundleManifest{}, errors.New("当前 Frida 安装包组件构建仅支持 Windows")
	}
	if err := recoverBundleDirectory(m.bundleRoot); err != nil {
		return BundleManifest{}, fmt.Errorf("恢复上一版 Frida 组件失败: %w", err)
	}
	release, err := m.Release(ctx, version)
	if err != nil {
		return BundleManifest{}, err
	}
	resolved := normalizeVersion(release.TagName)
	m.emitProgress(Progress{Phase: "release", Stage: "ready", Version: resolved, Message: "已锁定官方 Frida " + resolved})

	parent := filepath.Dir(m.bundleRoot)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return BundleManifest{}, fmt.Errorf("创建 Frida 组件父目录失败: %w", err)
	}
	stagingRoot, err := os.MkdirTemp(parent, "."+filepath.Base(m.bundleRoot)+"-update-*")
	if err != nil {
		return BundleManifest{}, fmt.Errorf("创建 Frida 更新暂存目录失败: %w", err)
	}
	defer func() {
		if stagingRoot != "" {
			_ = os.RemoveAll(stagingRoot)
		}
	}()
	if err := seedBundleLaunchers(m.bundleRoot, stagingRoot); err != nil {
		return BundleManifest{}, fmt.Errorf("准备 Frida 启动器失败: %w", err)
	}

	worker := &Manager{
		root:       m.root,
		bundleRoot: stagingRoot,
		client:     m.client,
	}
	worker.SetProgressEmitter(m.emitProgress)
	manifest, err := worker.installBundleRelease(ctx, release)
	if err != nil {
		return BundleManifest{}, err
	}
	if err := validateBundleDirectory(ctx, stagingRoot, manifest); err != nil {
		return BundleManifest{}, fmt.Errorf("Frida 更新暂存目录校验失败: %w", err)
	}
	if err := activateBundleDirectory(stagingRoot, m.bundleRoot); err != nil {
		return BundleManifest{}, fmt.Errorf("切换 Frida 组件版本失败，旧版本已保留: %w", err)
	}
	stagingRoot = ""
	m.emitProgress(Progress{
		Phase:   "runtime",
		Stage:   "complete",
		Version: resolved,
		Percent: 100,
		Message: fmt.Sprintf("Frida %s 组件更新完成", resolved),
		Done:    true,
	})
	return manifest, nil
}

func (m *Manager) installBundleRelease(ctx context.Context, release Release) (BundleManifest, error) {
	resolved := normalizeVersion(release.TagName)
	pythonPath, err := m.ensureBundlePython(ctx)
	if err != nil {
		return BundleManifest{}, err
	}
	m.emitProgress(Progress{Phase: "host", Stage: "installing", Version: resolved, Message: "正在安装内置 Frida CLI " + resolved})
	if err := runHidden(ctx, pythonPath, "-m", "pip", "install", "--disable-pip-version-check", "--no-input", "--no-warn-script-location", "--upgrade", "setuptools", "wheel"); err != nil {
		return BundleManifest{}, fmt.Errorf("准备内置 Frida CLI 构建依赖失败: %w", err)
	}
	if err := runHidden(ctx, pythonPath, "-m", "pip", "install", "--disable-pip-version-check", "--no-input", "--no-warn-script-location", "--no-build-isolation", "--upgrade", "frida=="+resolved, "frida-tools"); err != nil {
		return BundleManifest{}, fmt.Errorf("安装内置 Frida CLI 失败: %w", err)
	}
	hostVersion, err := bundlePythonValue(ctx, pythonPath, "import frida; print(frida.__version__)")
	if err != nil || normalizeVersion(hostVersion) != resolved {
		return BundleManifest{}, fmt.Errorf("内置 Frida CLI 版本校验失败: expected %s, got %s (%v)", resolved, hostVersion, err)
	}
	toolsVersion, _ := bundlePythonValue(ctx, pythonPath, "import importlib.metadata as m; print(m.version('frida-tools'))")
	pythonVersion, _ := bundlePythonValue(ctx, pythonPath, "import platform; print(platform.python_version())")

	manifest := BundleManifest{
		SchemaVersion:     bundleSchemaVersion,
		Version:           resolved,
		FridaToolsVersion: toolsVersion,
		PythonVersion:     pythonVersion,
		ReleaseURL:        release.HTMLURL,
		BuiltAt:           time.Now().UTC().Format(time.RFC3339),
		Servers:           make(map[string]BundleFile, len(androidServerArchitectures)),
	}
	for _, arch := range androidServerArchitectures {
		cachedPath, err := m.EnsureAndroidServer(ctx, release, arch)
		if err != nil {
			return BundleManifest{}, err
		}
		destination := bundleServerPath(m.bundleRoot, arch)
		if err := copyFileAtomic(cachedPath, destination, 0o755); err != nil {
			return BundleManifest{}, fmt.Errorf("写入 Android %s frida-server 失败: %w", arch, err)
		}
		metadata, _ := readMetadata(cachedPath)
		sha, err := fileSHA256(destination)
		if err != nil {
			return BundleManifest{}, err
		}
		manifest.Servers[arch] = BundleFile{
			Path:        filepath.ToSlash(filepath.Join("server", "android-"+arch, "frida-server")),
			SHA256:      sha,
			AssetSHA256: metadata.AssetSHA256,
			SourceURL:   metadata.URL,
		}
	}
	if err := writeBundleManifest(filepath.Join(m.bundleRoot, bundleManifestName), manifest); err != nil {
		return BundleManifest{}, err
	}
	return manifest, nil
}

func seedBundleLaunchers(sourceRoot, destinationRoot string) error {
	for _, name := range bundleLauncherFiles {
		source := filepath.Join(sourceRoot, name)
		info, err := os.Stat(source)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.IsDir() {
			return fmt.Errorf("Frida 启动器路径不是文件: %s", source)
		}
		if err := copyFileAtomic(source, filepath.Join(destinationRoot, name), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func validateBundleDirectory(ctx context.Context, root string, manifest BundleManifest) error {
	pythonPath := filepath.Join(root, "python", executableName("python"))
	version, err := bundlePythonValue(ctx, pythonPath, "import frida; print(frida.__version__)")
	if err != nil || normalizeVersion(version) != manifest.Version {
		return fmt.Errorf("内置 Frida Python 模块版本不一致: expected %s, got %s (%v)", manifest.Version, version, err)
	}
	for _, arch := range androidServerArchitectures {
		entry, ok := manifest.Servers[arch]
		if !ok {
			return fmt.Errorf("组件清单缺少 Android %s frida-server", arch)
		}
		if err := verifyBundleFile(root, entry); err != nil {
			return err
		}
	}
	launcher := bundleExecutablePath(root, "frida")
	if bundleFileExists(launcher) {
		launcherVersion := normalizeVersion(commandVersion(ctx, launcher))
		if launcherVersion != manifest.Version {
			return fmt.Errorf("内置 Frida 启动器版本不一致: expected %s, got %s", manifest.Version, launcherVersion)
		}
	}
	return nil
}

func recoverBundleDirectory(target string) error {
	target = filepath.Clean(target)
	journalPath := bundleSwapJournalPath(target)
	journal, err := readBundleSwapJournal(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	backup := filepath.Join(filepath.Dir(target), journal.BackupName)
	targetInfo, targetErr := os.Stat(target)
	if targetErr == nil {
		if !targetInfo.IsDir() {
			return fmt.Errorf("Frida 组件路径不是目录: %s", target)
		}
		if backupInfo, backupErr := os.Stat(backup); backupErr == nil {
			if !backupInfo.IsDir() {
				return fmt.Errorf("Frida 组件备份路径不是目录: %s", backup)
			}
			if err := os.RemoveAll(backup); err != nil {
				return err
			}
		} else if !errors.Is(backupErr, os.ErrNotExist) {
			return backupErr
		}
		return os.Remove(journalPath)
	}
	if !errors.Is(targetErr, os.ErrNotExist) {
		return targetErr
	}
	backupInfo, err := os.Stat(backup)
	if err != nil {
		return fmt.Errorf("Frida 更新日志存在，但备份目录不可用: %w", err)
	}
	if !backupInfo.IsDir() {
		return fmt.Errorf("Frida 组件备份路径不是目录: %s", backup)
	}
	if err := os.Rename(backup, target); err != nil {
		return err
	}
	return os.Remove(journalPath)
}

func activateBundleDirectory(staging, target string) error {
	staging = filepath.Clean(staging)
	target = filepath.Clean(target)
	if filepath.Dir(staging) != filepath.Dir(target) {
		return errors.New("Frida 暂存目录必须与安装目录位于同一父目录")
	}
	if err := recoverBundleDirectory(target); err != nil {
		return err
	}
	targetExists := false
	if info, err := os.Stat(target); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("Frida 组件路径不是目录: %s", target)
		}
		targetExists = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !targetExists {
		return os.Rename(staging, target)
	}

	parent := filepath.Dir(target)
	backup, err := os.MkdirTemp(parent, "."+filepath.Base(target)+"-previous-*")
	if err != nil {
		return err
	}
	if err := os.Remove(backup); err != nil {
		return err
	}
	if err := writeBundleSwapJournal(target, backup); err != nil {
		return err
	}
	journalPath := bundleSwapJournalPath(target)
	if err := os.Rename(target, backup); err != nil {
		_ = os.Remove(journalPath)
		return err
	}
	if err := os.Rename(staging, target); err != nil {
		if restoreErr := os.Rename(backup, target); restoreErr == nil {
			_ = os.Remove(journalPath)
		}
		return err
	}
	if err := os.RemoveAll(backup); err == nil {
		_ = os.Remove(journalPath)
	}
	return nil
}

func bundleSwapJournalPath(target string) string {
	return filepath.Clean(target) + ".update-journal"
}

func writeBundleSwapJournal(target, backup string) error {
	target = filepath.Clean(target)
	backup = filepath.Clean(backup)
	parent := filepath.Dir(target)
	backupName := filepath.Base(backup)
	prefix := "." + filepath.Base(target) + "-previous-"
	if filepath.Dir(backup) != parent || !strings.HasPrefix(backupName, prefix) || backupName == prefix {
		return errors.New("Frida 备份目录不符合安全约束")
	}
	data, err := json.Marshal(bundleSwapJournal{Magic: bundleSwapJournalMagic, BackupName: backupName})
	if err != nil {
		return err
	}
	journalPath := bundleSwapJournalPath(target)
	journal, err := os.OpenFile(journalPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	keepJournal := false
	defer func() {
		if !keepJournal {
			_ = os.Remove(journalPath)
		}
	}()
	if _, err := journal.Write(append(data, '\n')); err != nil {
		journal.Close()
		return err
	}
	if err := journal.Sync(); err != nil {
		journal.Close()
		return err
	}
	if err := journal.Close(); err != nil {
		return err
	}
	keepJournal = true
	return nil
}

func readBundleSwapJournal(target string) (bundleSwapJournal, error) {
	data, err := os.ReadFile(bundleSwapJournalPath(target))
	if err != nil {
		return bundleSwapJournal{}, err
	}
	if len(data) > 4096 {
		return bundleSwapJournal{}, errors.New("Frida 更新日志异常：文件过大")
	}
	var journal bundleSwapJournal
	if err := json.Unmarshal(data, &journal); err != nil {
		return bundleSwapJournal{}, fmt.Errorf("解析 Frida 更新日志失败: %w", err)
	}
	prefix := "." + filepath.Base(filepath.Clean(target)) + "-previous-"
	if journal.Magic != bundleSwapJournalMagic || filepath.Base(journal.BackupName) != journal.BackupName ||
		!strings.HasPrefix(journal.BackupName, prefix) || journal.BackupName == prefix {
		return bundleSwapJournal{}, errors.New("Frida 更新日志不符合安全约束")
	}
	return journal, nil
}

func (m *Manager) ensureBundlePython(ctx context.Context) (string, error) {
	root := filepath.Join(m.bundleRoot, "python")
	pythonPath := filepath.Join(root, executableName("python"))
	if !bundleFileExists(pythonPath) {
		if err := os.MkdirAll(root, 0o755); err != nil {
			return "", err
		}
		archiveFile, err := os.CreateTemp(m.root, ".python-embed-*.zip")
		if err != nil {
			if mkdirErr := os.MkdirAll(m.root, 0o700); mkdirErr != nil {
				return "", mkdirErr
			}
			archiveFile, err = os.CreateTemp(m.root, ".python-embed-*.zip")
		}
		if err != nil {
			return "", err
		}
		archivePath := archiveFile.Name()
		defer os.Remove(archivePath)
		m.emitProgress(Progress{Phase: "python", Stage: "downloading", Asset: filepath.Base(pythonEmbedDownload), Message: "正在下载安装包内置 Python"})
		allowed := func(url string) bool { return strings.HasPrefix(url, "https://www.python.org/") }
		if err := m.downloadURL(ctx, pythonEmbedDownload, archiveFile, allowed, func(downloaded, total int64) {
			m.emitDownloadProgress("python", "downloading", "", filepath.Base(pythonEmbedDownload), downloaded, total, "正在下载安装包内置 Python")
		}); err != nil {
			archiveFile.Close()
			return "", fmt.Errorf("下载安装包内置 Python 失败: %w", err)
		}
		if err := archiveFile.Close(); err != nil {
			return "", err
		}
		archive, err := zip.OpenReader(archivePath)
		if err != nil {
			return "", err
		}
		extractErr := extractZipSafe(root, &archive.Reader)
		closeErr := archive.Close()
		if extractErr != nil || closeErr != nil {
			return "", errors.Join(extractErr, closeErr)
		}
	}
	if err := enableEmbeddedSitePackages(root); err != nil {
		return "", err
	}
	if _, err := runOutput(ctx, pythonPath, "-m", "pip", "--version"); err != nil {
		pipFile, createErr := os.CreateTemp(m.root, ".get-pip-*.py")
		if createErr != nil {
			return "", createErr
		}
		pipPath := pipFile.Name()
		defer os.Remove(pipPath)
		allowed := func(url string) bool { return strings.HasPrefix(url, "https://bootstrap.pypa.io/") }
		m.emitProgress(Progress{Phase: "python", Stage: "downloading", Asset: filepath.Base(pipBootstrapURL), Message: "正在初始化内置 Python pip"})
		if err := m.downloadURL(ctx, pipBootstrapURL, pipFile, allowed); err != nil {
			pipFile.Close()
			return "", fmt.Errorf("下载 pip 引导程序失败: %w", err)
		}
		if err := pipFile.Close(); err != nil {
			return "", err
		}
		if err := runHidden(ctx, pythonPath, pipPath, "--disable-pip-version-check"); err != nil {
			return "", fmt.Errorf("初始化内置 Python pip 失败: %w", err)
		}
	}
	return pythonPath, nil
}

func bundlePythonValue(ctx context.Context, pythonPath string, source string) (string, error) {
	output, err := runOutput(ctx, pythonPath, "-c", source)
	return firstLine(output), err
}

func executableName(name string) string {
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		return name + ".exe"
	}
	return name
}

func bundleExecutablePath(root string, name string) string {
	return filepath.Join(root, executableName(name))
}

func bundleServerPath(root string, arch string) string {
	return filepath.Join(root, "server", "android-"+arch, "frida-server")
}

func verifyBundleFile(root string, entry BundleFile) error {
	rel := filepath.Clean(filepath.FromSlash(entry.Path))
	if rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("Frida 组件清单包含不安全路径")
	}
	path := filepath.Join(root, rel)
	if !bundleFileExists(path) {
		return fmt.Errorf("Frida 组件文件缺失: %s", path)
	}
	actual, err := fileSHA256(path)
	if err != nil {
		return err
	}
	if entry.SHA256 == "" || !strings.EqualFold(actual, digestValue(entry.SHA256)) {
		return fmt.Errorf("Frida 组件 SHA-256 校验失败: %s", path)
	}
	return nil
}

func writeBundleManifest(path string, manifest BundleManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFileAtomic(path, data, 0o644)
}

func copyFileAtomic(source string, destination string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(destination), ".component-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return err
	}
	if _, err := io.Copy(temp, input); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return replaceFile(tempPath, destination)
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".manifest-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return replaceFile(tempPath, path)
}

func replaceFile(source string, destination string) error {
	backup := destination + ".previous"
	_ = os.Remove(backup)
	hadDestination := false
	if _, err := os.Stat(destination); err == nil {
		if err := os.Rename(destination, backup); err != nil {
			return err
		}
		hadDestination = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(source, destination); err != nil {
		if hadDestination {
			_ = os.Rename(backup, destination)
		}
		return err
	}
	if hadDestination {
		_ = os.Remove(backup)
	}
	return nil
}

func bundleFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}
