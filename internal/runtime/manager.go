package runtime

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"frida-gui-helper/internal/processutil"
	"frida-gui-helper/internal/toolchain"
	"github.com/ulikunitz/xz"
)

const (
	githubAPIBase       = "https://api.github.com/repos/frida/frida"
	platformToolsURL    = "https://dl.google.com/android/repository/platform-tools-latest-windows.zip"
	pythonEmbedVersion  = "3.13.5"
	pythonEmbedDownload = "https://www.python.org/ftp/python/3.13.5/python-3.13.5-embed-amd64.zip"
	pipBootstrapURL     = "https://bootstrap.pypa.io/get-pip.py"
)

type ReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
	Size               int64  `json:"size"`
}

type Release struct {
	TagName string         `json:"tag_name"`
	HTMLURL string         `json:"html_url"`
	Assets  []ReleaseAsset `json:"assets"`
}

type ToolStatus struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Found   bool   `json:"found"`
	Source  string `json:"source"`
	Version string `json:"version"`
	Error   string `json:"error"`
}

type Status struct {
	LatestVersion string     `json:"latestVersion"`
	CacheDir      string     `json:"cacheDir"`
	Host          ToolStatus `json:"host"`
	Python        ToolStatus `json:"python"`
	Server        ToolStatus `json:"server"`
}

type Progress struct {
	Phase           string  `json:"phase"`
	Stage           string  `json:"stage"`
	Version         string  `json:"version"`
	Asset           string  `json:"asset"`
	BytesDownloaded int64   `json:"bytesDownloaded"`
	TotalBytes      int64   `json:"totalBytes"`
	Percent         float64 `json:"percent"`
	Message         string  `json:"message"`
	Done            bool    `json:"done"`
	Error           string  `json:"error"`
}

type ProgressEmitter func(Progress)

type DownloadProgress func(downloaded int64, total int64)

type Manager struct {
	root         string
	bundleRoot   string
	tools        *toolchain.Resolver
	client       *http.Client
	bundleMu     sync.RWMutex
	progressMu   sync.RWMutex
	progressEmit ProgressEmitter
}

func NewManager(tools *toolchain.Resolver) *Manager {
	root, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(root) == "" {
		root = os.TempDir()
	}
	root = filepath.Join(root, "Frida-GUI-Helper", "runtime")
	bundleRoot := "frida"
	if tools != nil {
		bundleRoot = tools.FridaRoot()
	}
	_ = recoverBundleDirectory(bundleRoot)
	return &Manager{
		root:       root,
		bundleRoot: bundleRoot,
		tools:      tools,
		client:     &http.Client{Timeout: 90 * time.Second},
	}
}

func (m *Manager) SetProgressEmitter(emit ProgressEmitter) {
	m.progressMu.Lock()
	defer m.progressMu.Unlock()
	m.progressEmit = emit
}

func (m *Manager) emitProgress(progress Progress) {
	m.progressMu.RLock()
	emit := m.progressEmit
	m.progressMu.RUnlock()
	if emit != nil {
		emit(progress)
	}
}

func (m *Manager) emitDownloadProgress(phase string, stage string, version string, asset string,
	downloaded int64, total int64, message string) {
	percent := float64(0)
	if total > 0 {
		percent = float64(downloaded) / float64(total) * 100
		if percent > 100 {
			percent = 100
		}
	}
	m.emitProgress(Progress{
		Phase:           phase,
		Stage:           stage,
		Version:         version,
		Asset:           asset,
		BytesDownloaded: downloaded,
		TotalBytes:      total,
		Percent:         percent,
		Message:         message,
	})
}

func (m *Manager) emitFailure(phase string, version string, asset string, message string) {
	m.emitProgress(Progress{
		Phase:   phase,
		Stage:   "failed",
		Version: version,
		Asset:   asset,
		Message: message,
		Done:    true,
		Error:   message,
	})
}

func (m *Manager) Root() string {
	return m.bundleRoot
}

// CacheRoot is reserved for temporary downloads and ADB platform-tools. Frida
// itself always lives below BundleRoot and is never resolved from PATH.
func (m *Manager) CacheRoot() string {
	return m.root
}

func (m *Manager) BundleRoot() string {
	return m.bundleRoot
}

// SetBundleRoot is primarily used by the reproducible installer build helper.
// Runtime callers normally use the frida directory beside the application.
func (m *Manager) SetBundleRoot(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("Frida 组件目录不能为空")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	m.bundleRoot = filepath.Clean(abs)
	return nil
}

func (m *Manager) EnsureADB(ctx context.Context) (ToolStatus, error) {
	if m.tools != nil {
		adb := m.tools.FindExecutable("adb")
		if adb.Found {
			return ToolStatus{Name: "adb", Path: adb.Path, Found: true, Source: adb.Source}, nil
		}
	}
	if runtime.GOOS != "windows" {
		return ToolStatus{Name: "adb"}, errors.New("未找到 adb；当前自动下载仅支持 Windows Platform-tools")
	}
	root := filepath.Join(m.root, "platform-tools")
	adbPath := filepath.Join(root, "platform-tools", "adb.exe")
	if _, err := os.Stat(adbPath); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(root, 0o700); err != nil {
			return ToolStatus{Name: "adb"}, err
		}
		archiveFile, err := os.CreateTemp(root, ".platform-tools-*.zip")
		if err != nil {
			return ToolStatus{Name: "adb"}, err
		}
		archivePath := archiveFile.Name()
		defer os.Remove(archivePath)
		allowed := func(url string) bool {
			return strings.HasPrefix(url, "https://dl.google.com/android/repository/")
		}
		m.emitProgress(Progress{
			Phase:   "adb",
			Stage:   "downloading",
			Asset:   filepath.Base(platformToolsURL),
			Message: "正在下载 Android Platform-tools",
		})
		if err := m.downloadURL(ctx, platformToolsURL, archiveFile, allowed, func(downloaded int64, total int64) {
			m.emitDownloadProgress("adb", "downloading", "", filepath.Base(platformToolsURL), downloaded, total,
				"正在下载 Android Platform-tools")
		}); err != nil {
			archiveFile.Close()
			wrapped := fmt.Errorf("下载 Android Platform-tools 失败: %w", err)
			m.emitFailure("adb", "", filepath.Base(platformToolsURL), wrapped.Error())
			return ToolStatus{Name: "adb"}, wrapped
		}
		if err := archiveFile.Close(); err != nil {
			return ToolStatus{Name: "adb"}, err
		}
		archive, err := zip.OpenReader(archivePath)
		if err != nil {
			return ToolStatus{Name: "adb"}, fmt.Errorf("打开 Android Platform-tools 压缩包失败: %w", err)
		}
		err = extractZipSafe(root, &archive.Reader)
		closeErr := archive.Close()
		if err != nil {
			return ToolStatus{Name: "adb"}, fmt.Errorf("解压 Android Platform-tools 失败: %w", err)
		}
		if closeErr != nil {
			return ToolStatus{Name: "adb"}, closeErr
		}
	}
	if m.tools != nil {
		m.tools.AddBaseDir(filepath.Join(root, "platform-tools"))
		adb := m.tools.FindExecutable("adb")
		if adb.Found {
			return ToolStatus{Name: "adb", Path: adb.Path, Found: true, Source: "managed"}, nil
		}
	}
	return ToolStatus{Name: "adb"}, fmt.Errorf("Platform-tools 已下载但未找到 adb.exe: %s", adbPath)
}

func (m *Manager) Latest(ctx context.Context) (Release, error) {
	return m.fetchRelease(ctx, githubAPIBase+"/releases/latest")
}

func (m *Manager) Release(ctx context.Context, version string) (Release, error) {
	version = normalizeVersion(version)
	if version == "" || version == "latest" {
		return m.Latest(ctx)
	}
	// Frida's public release tags are normally numeric (for example
	// "17.16.4"). Keep a v-prefixed fallback for older or mirrored tags.
	release, err := m.fetchRelease(ctx, githubAPIBase+"/releases/tags/"+version)
	if err == nil {
		return release, nil
	}
	return m.fetchRelease(ctx, githubAPIBase+"/releases/tags/v"+version)
}

func (m *Manager) EnsureAndroidServer(ctx context.Context, release Release, abi string) (string, error) {
	version := normalizeVersion(release.TagName)
	arch, err := normalizeAndroidArch(abi)
	if err != nil {
		return "", err
	}
	asset, err := findServerAsset(release.Assets, version, arch)
	if err != nil {
		return "", err
	}
	destination := filepath.Join(m.root, "servers", version, arch, "frida-server")
	if ok, err := verifiedServerCache(destination, asset, version, arch); err == nil && ok {
		m.emitProgress(Progress{
			Phase:   "server",
			Stage:   "cached",
			Version: version,
			Asset:   asset.Name,
			Percent: 100,
			Message: fmt.Sprintf("已复用缓存的 Android frida-server (%s)", arch),
		})
		return destination, nil
	}

	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return "", fmt.Errorf("创建 Frida server 缓存目录失败: %w", err)
	}
	tempXZ, err := os.CreateTemp(filepath.Dir(destination), ".frida-server-*.xz")
	if err != nil {
		return "", err
	}
	tempXZPath := tempXZ.Name()
	defer os.Remove(tempXZPath)
	m.emitProgress(Progress{
		Phase:   "server",
		Stage:   "downloading",
		Version: version,
		Asset:   asset.Name,
		Message: fmt.Sprintf("正在下载 Android frida-server (%s)", arch),
	})
	if err := m.download(ctx, asset.BrowserDownloadURL, tempXZ, func(downloaded int64, total int64) {
		m.emitDownloadProgress("server", "downloading", version, asset.Name, downloaded, total,
			fmt.Sprintf("正在下载 Android frida-server (%s)", arch))
	}); err != nil {
		tempXZ.Close()
		wrapped := fmt.Errorf("下载 frida-server 失败: %w", err)
		m.emitFailure("server", version, asset.Name, wrapped.Error())
		return "", wrapped
	}
	if err := tempXZ.Close(); err != nil {
		return "", err
	}

	compressedSHA, err := fileSHA256(tempXZPath)
	if err != nil {
		return "", fmt.Errorf("计算 frida-server 下载包 SHA-256 失败: %w", err)
	}
	expectedAssetSHA := digestValue(asset.Digest)
	if !validSHA256(expectedAssetSHA) {
		wrapped := fmt.Errorf("官方 Release 未提供有效的 frida-server SHA-256: %s", asset.Name)
		m.emitFailure("server", version, asset.Name, wrapped.Error())
		return "", wrapped
	}
	if !strings.EqualFold(compressedSHA, expectedAssetSHA) {
		wrapped := fmt.Errorf("frida-server 下载包 SHA-256 校验失败: expected %s, got %s", expectedAssetSHA, compressedSHA)
		m.emitFailure("server", version, asset.Name, wrapped.Error())
		return "", wrapped
	}
	m.emitProgress(Progress{
		Phase:   "server",
		Stage:   "verifying",
		Version: version,
		Asset:   asset.Name,
		Percent: 100,
		Message: "下载包 SHA-256 校验通过，准备解压",
	})

	input, err := os.Open(tempXZPath)
	if err != nil {
		return "", err
	}
	reader, err := xz.NewReader(input)
	if err != nil {
		input.Close()
		return "", fmt.Errorf("解压 frida-server 失败: %w", err)
	}
	tempBin, err := os.CreateTemp(filepath.Dir(destination), ".frida-server-*.bin")
	if err != nil {
		input.Close()
		return "", err
	}
	tempBinPath := tempBin.Name()
	defer os.Remove(tempBinPath)
	m.emitProgress(Progress{
		Phase:   "server",
		Stage:   "extracting",
		Version: version,
		Asset:   asset.Name,
		Percent: 100,
		Message: "正在解压 Android frida-server",
	})
	if _, err := io.Copy(tempBin, reader); err != nil {
		tempBin.Close()
		input.Close()
		return "", fmt.Errorf("写入解压后的 frida-server 失败: %w", err)
	}
	if err := tempBin.Sync(); err != nil {
		tempBin.Close()
		input.Close()
		return "", err
	}
	if err := tempBin.Close(); err != nil {
		input.Close()
		return "", err
	}
	if err := input.Close(); err != nil {
		return "", err
	}

	actual, err := fileSHA256(tempBinPath)
	if err != nil {
		return "", err
	}
	if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.Rename(tempBinPath, destination); err != nil {
		return "", fmt.Errorf("缓存 frida-server 失败: %w", err)
	}
	if err := writeMetadata(destination, runtimeMetadata{
		Version:     version,
		Arch:        arch,
		SHA256:      actual,
		FileSHA256:  actual,
		AssetSHA256: compressedSHA,
		URL:         asset.BrowserDownloadURL,
	}); err != nil {
		return "", err
	}
	m.emitProgress(Progress{
		Phase:   "server",
		Stage:   "complete",
		Version: version,
		Asset:   asset.Name,
		Percent: 100,
		Message: fmt.Sprintf("Android frida-server 已准备完成 (%s)", arch),
	})
	return destination, nil
}

func (m *Manager) EnsureHost(ctx context.Context, version string) (ToolStatus, error) {
	version = normalizeVersion(version)
	if version == "" {
		return ToolStatus{Name: "frida"}, errors.New("Frida 版本为空")
	}
	m.emitProgress(Progress{
		Phase:   "host",
		Stage:   "checking",
		Version: version,
		Message: fmt.Sprintf("正在检查本机 Frida CLI %s", version),
	})
	if m.tools != nil {
		if status, ok := findHost(m.tools, ctx, version); ok {
			m.emitProgress(Progress{
				Phase:   "host",
				Stage:   "cached",
				Version: version,
				Percent: 100,
				Message: fmt.Sprintf("已复用本机 Frida CLI %s", version),
			})
			return status, nil
		}
	}

	m.emitProgress(Progress{
		Phase:   "host",
		Stage:   "installing",
		Version: version,
		Message: fmt.Sprintf("正在安装 Frida CLI %s", version),
	})
	python, err := m.findOrInstallPython(ctx)
	if err != nil {
		return ToolStatus{Name: "frida"}, err
	}
	hostRoot := filepath.Join(m.root, "hosts", version)
	if python.Name == "managed-python" {
		m.emitProgress(Progress{
			Phase:   "host",
			Stage:   "installing",
			Version: version,
			Message: fmt.Sprintf("正在通过 Python 安装 Frida CLI %s", version),
		})
		if err := runHidden(ctx, python.Path, "-m", "pip", "install", "--disable-pip-version-check", "--no-input", "--upgrade", "frida=="+version, "frida-tools"); err != nil {
			return ToolStatus{Name: "frida"}, fmt.Errorf("安装 Frida CLI 失败: %w", err)
		}
		if m.tools != nil {
			m.tools.AddBaseDir(filepath.Join(filepath.Dir(python.Path), "Scripts"))
		}
		status, ok := findHost(m.tools, ctx, version)
		if !ok {
			return status, fmt.Errorf("Frida CLI 安装完成但未找到 frida.exe/frida-ps.exe")
		}
		return status, nil
	}
	venv := filepath.Join(hostRoot, "venv")
	venvPython := filepath.Join(venv, "Scripts", "python.exe")
	if runtime.GOOS != "windows" {
		venvPython = filepath.Join(venv, "bin", "python")
	}
	if _, err := os.Stat(venvPython); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(hostRoot, 0o700); err != nil {
			return ToolStatus{Name: "frida"}, err
		}
		args := []string{"-m", "venv", venv}
		if python.Name == "py" {
			args = append([]string{"-3"}, args...)
		}
		if err := runHidden(ctx, python.Path, args...); err != nil {
			return ToolStatus{Name: "frida"}, fmt.Errorf("创建 Frida Python 环境失败: %w", err)
		}
	}
	if _, err := os.Stat(venvPython); err != nil {
		return ToolStatus{Name: "frida"}, fmt.Errorf("Python 虚拟环境未生成: %w", err)
	}
	m.emitProgress(Progress{
		Phase:   "host",
		Stage:   "installing",
		Version: version,
		Message: fmt.Sprintf("正在通过 Python 安装 Frida CLI %s", version),
	})
	if err := runHidden(ctx, venvPython, "-m", "pip", "install", "--disable-pip-version-check", "--no-input", "--upgrade", "frida=="+version, "frida-tools"); err != nil {
		return ToolStatus{Name: "frida"}, fmt.Errorf("安装 Frida CLI 失败: %w", err)
	}
	binDir := filepath.Dir(venvPython)
	if m.tools != nil {
		m.tools.AddBaseDir(binDir)
	}
	status, ok := findHost(m.tools, ctx, version)
	if !ok {
		return status, fmt.Errorf("Frida CLI 安装完成但未找到 frida.exe/frida-ps.exe")
	}
	return status, nil
}

func (m *Manager) Prepare(ctx context.Context, version string, abi string) (Status, error) {
	requestedVersion := normalizeVersion(version)
	installedVersion := m.installedHostVersion(ctx)
	resolvedVersion := resolveRuntimeVersion(requestedVersion, installedVersion)
	m.emitProgress(Progress{
		Phase:   "release",
		Stage:   "resolving",
		Version: resolvedVersion,
		Message: runtimeVersionMessage(requestedVersion, installedVersion, resolvedVersion),
	})
	release, err := m.Release(ctx, resolvedVersion)
	if err != nil {
		return Status{CacheDir: m.root}, err
	}
	normalized := normalizeVersion(release.TagName)
	m.emitProgress(Progress{
		Phase:   "release",
		Stage:   "ready",
		Version: normalized,
		Message: fmt.Sprintf("已选择 Frida %s 官方 Release", normalized),
	})
	host, hostErr := m.EnsureHost(ctx, normalized)
	serverPath, serverErr := m.EnsureAndroidServer(ctx, release, abi)
	status := Status{
		LatestVersion: normalized,
		CacheDir:      m.root,
		Host:          host,
		Server:        ToolStatus{Name: "frida-server", Path: serverPath, Found: serverErr == nil, Source: "managed", Version: normalized},
	}
	if hostErr != nil {
		status.Host.Error = hostErr.Error()
	}
	if serverErr != nil {
		status.Server.Error = serverErr.Error()
	}
	if hostErr != nil || serverErr != nil {
		m.emitProgress(Progress{
			Phase:   "runtime",
			Stage:   "failed",
			Version: normalized,
			Message: "Frida 环境配置失败",
			Done:    true,
			Error:   errors.Join(hostErr, serverErr).Error(),
		})
		return status, errors.Join(hostErr, serverErr)
	}
	m.emitProgress(Progress{
		Phase:   "runtime",
		Stage:   "complete",
		Version: normalized,
		Percent: 100,
		Message: fmt.Sprintf("Frida %s 环境准备完成", normalized),
		Done:    true,
	})
	return status, nil
}

func (m *Manager) installedHostVersion(ctx context.Context) string {
	if m.tools == nil {
		return ""
	}
	tool := m.tools.FindFridaTool("frida")
	if !tool.Found {
		return ""
	}
	version := normalizeVersion(commandVersion(ctx, tool.Path))
	if version == "" || version == "latest" || strings.ContainsAny(version, `/\\`) {
		return ""
	}
	return version
}

func resolveRuntimeVersion(requested string, installed string) string {
	requested = normalizeVersion(requested)
	installed = normalizeVersion(installed)
	if requested == "" || requested == "latest" {
		if installed != "" && installed != "latest" {
			return installed
		}
		return "latest"
	}
	return requested
}

func runtimeVersionMessage(requested string, installed string, resolved string) string {
	if (requested == "" || requested == "latest") && installed != "" && resolved == installed {
		return fmt.Sprintf("检测到本机 Frida CLI %s，将下载同版本 Android frida-server", installed)
	}
	if resolved == "latest" {
		return "未检测到本机 Frida CLI，将获取官方最新版"
	}
	return fmt.Sprintf("准备 Frida %s 环境", resolved)
}

func (m *Manager) fetchRelease(ctx context.Context, endpoint string) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Frida-GUI-Helper")
	response, err := m.client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub Release 请求失败: HTTP %d", response.StatusCode)
	}
	var release Release
	if err := json.NewDecoder(response.Body).Decode(&release); err != nil {
		return Release{}, fmt.Errorf("解析 GitHub Release 失败: %w", err)
	}
	if normalizeVersion(release.TagName) == "" {
		return Release{}, errors.New("GitHub Release 未返回有效版本")
	}
	return release, nil
}

func (m *Manager) download(ctx context.Context, url string, destination io.Writer, progress ...DownloadProgress) error {
	if !isGitHubDownloadURL(url) {
		return errors.New("拒绝下载非 GitHub HTTPS 地址")
	}
	return m.downloadURL(ctx, url, destination, isGitHubDownloadURL, progress...)
}

func (m *Manager) downloadURL(ctx context.Context, url string, destination io.Writer,
	allowed func(string) bool, progress ...DownloadProgress) error {
	if !allowed(url) {
		return errors.New("拒绝下载未允许的 HTTPS 地址")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Frida-GUI-Helper")
	// The GitHub asset digest covers the exact uploaded bytes. Avoid content
	// encoding transformations before calculating the SHA-256 of the .xz file.
	req.Header.Set("Accept-Encoding", "identity")
	response, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("下载请求失败: HTTP %d", response.StatusCode)
	}
	var callback DownloadProgress
	if len(progress) > 0 {
		callback = progress[0]
	}
	tracker := newDownloadTracker(destination, response.ContentLength, callback)
	tracker.report(true)
	_, err = io.CopyBuffer(tracker, response.Body, make([]byte, 64*1024))
	tracker.report(true)
	return err
}

type downloadTracker struct {
	destination io.Writer
	total       int64
	downloaded  int64
	lastBytes   int64
	lastAt      time.Time
	callback    DownloadProgress
}

func newDownloadTracker(destination io.Writer, total int64, callback DownloadProgress) *downloadTracker {
	return &downloadTracker{
		destination: destination,
		total:       total,
		lastAt:      time.Now(),
		callback:    callback,
	}
}

func (w *downloadTracker) Write(p []byte) (int, error) {
	n, err := w.destination.Write(p)
	w.downloaded += int64(n)
	if err != nil {
		return n, err
	}
	if w.downloaded-w.lastBytes >= 256*1024 || time.Since(w.lastAt) >= 150*time.Millisecond {
		w.report(false)
	}
	return n, nil
}

func (w *downloadTracker) report(force bool) {
	if w.callback == nil {
		return
	}
	if !force && w.downloaded == w.lastBytes {
		return
	}
	w.lastBytes = w.downloaded
	w.lastAt = time.Now()
	w.callback(w.downloaded, w.total)
}

func (m *Manager) findOrInstallPython(ctx context.Context) (ToolStatus, error) {
	if m.tools != nil {
		tool := m.tools.FindBundledPython()
		if tool.Found {
			return ToolStatus{Name: "python", Path: tool.Path, Found: true, Source: tool.Source}, nil
		}
	}
	if runtime.GOOS == "windows" {
		return m.installEmbeddedPython(ctx)
	}
	return ToolStatus{Name: "python"}, errors.New("未找到 Python。请安装 Python 3，或将便携 Python 加入后重试；Frida 官方 CLI 通过 Python 发布")
}

func (m *Manager) installEmbeddedPython(ctx context.Context) (ToolStatus, error) {
	root := filepath.Join(m.root, "python", pythonEmbedVersion)
	pythonPath := filepath.Join(root, "python.exe")
	if _, err := os.Stat(pythonPath); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(root, 0o700); err != nil {
			return ToolStatus{Name: "python"}, err
		}
		archiveFile, err := os.CreateTemp(root, ".python-embed-*.zip")
		if err != nil {
			return ToolStatus{Name: "python"}, err
		}
		archivePath := archiveFile.Name()
		defer os.Remove(archivePath)
		allowed := func(url string) bool {
			return strings.HasPrefix(url, "https://www.python.org/")
		}
		m.emitProgress(Progress{
			Phase:   "python",
			Stage:   "downloading",
			Asset:   filepath.Base(pythonEmbedDownload),
			Message: "正在下载便携 Python",
		})
		if err := m.downloadURL(ctx, pythonEmbedDownload, archiveFile, allowed, func(downloaded int64, total int64) {
			m.emitDownloadProgress("python", "downloading", "", filepath.Base(pythonEmbedDownload), downloaded, total,
				"正在下载便携 Python")
		}); err != nil {
			archiveFile.Close()
			wrapped := fmt.Errorf("下载便携 Python 失败: %w", err)
			m.emitFailure("python", "", filepath.Base(pythonEmbedDownload), wrapped.Error())
			return ToolStatus{Name: "python"}, wrapped
		}
		if err := archiveFile.Close(); err != nil {
			return ToolStatus{Name: "python"}, err
		}
		archive, err := zip.OpenReader(archivePath)
		if err != nil {
			return ToolStatus{Name: "python"}, fmt.Errorf("打开便携 Python 压缩包失败: %w", err)
		}
		err = extractZipSafe(root, &archive.Reader)
		closeErr := archive.Close()
		if err != nil {
			return ToolStatus{Name: "python"}, fmt.Errorf("解压便携 Python 失败: %w", err)
		}
		if closeErr != nil {
			return ToolStatus{Name: "python"}, closeErr
		}
	}
	if _, err := os.Stat(pythonPath); err != nil {
		return ToolStatus{Name: "python"}, fmt.Errorf("便携 Python 未生成: %w", err)
	}
	if err := enableEmbeddedSitePackages(root); err != nil {
		return ToolStatus{Name: "python"}, err
	}
	if _, err := runOutput(ctx, pythonPath, "-m", "pip", "--version"); err != nil {
		pipFile, createErr := os.CreateTemp(root, "get-pip-*.py")
		if createErr != nil {
			return ToolStatus{Name: "python"}, createErr
		}
		pipPath := pipFile.Name()
		defer os.Remove(pipPath)
		allowed := func(url string) bool {
			return strings.HasPrefix(url, "https://bootstrap.pypa.io/")
		}
		m.emitProgress(Progress{
			Phase:   "python",
			Stage:   "downloading",
			Asset:   filepath.Base(pipBootstrapURL),
			Message: "正在下载 pip 引导程序",
		})
		if downloadErr := m.downloadURL(ctx, pipBootstrapURL, pipFile, allowed, func(downloaded int64, total int64) {
			m.emitDownloadProgress("python", "downloading", "", filepath.Base(pipBootstrapURL), downloaded, total,
				"正在下载 pip 引导程序")
		}); downloadErr != nil {
			pipFile.Close()
			wrapped := fmt.Errorf("下载 pip 引导程序失败: %w", downloadErr)
			m.emitFailure("python", "", filepath.Base(pipBootstrapURL), wrapped.Error())
			return ToolStatus{Name: "python"}, wrapped
		}
		if closeErr := pipFile.Close(); closeErr != nil {
			return ToolStatus{Name: "python"}, closeErr
		}
		if runErr := runHidden(ctx, pythonPath, pipPath, "--disable-pip-version-check"); runErr != nil {
			return ToolStatus{Name: "python"}, fmt.Errorf("初始化便携 Python pip 失败: %w", runErr)
		}
	}
	if m.tools != nil {
		m.tools.AddBaseDir(root)
	}
	return ToolStatus{Name: "managed-python", Path: pythonPath, Found: true, Source: "managed"}, nil
}

func enableEmbeddedSitePackages(root string) error {
	paths, err := filepath.Glob(filepath.Join(root, "*._pth"))
	if err != nil {
		return err
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
		enabled := false
		for index, line := range lines {
			if strings.TrimSpace(line) == "#import site" || strings.TrimSpace(line) == "import site" {
				lines[index] = "import site"
				enabled = true
			}
		}
		if !enabled {
			lines = append(lines, "import site")
		}
		text := strings.TrimRight(strings.Join(lines, "\r\n"), "\r\n") + "\r\n"
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func findHost(tools *toolchain.Resolver, ctx context.Context, expected string) (ToolStatus, bool) {
	if tools == nil {
		return ToolStatus{Name: "frida"}, false
	}
	fridaTool := tools.FindFridaTool("frida")
	psTool := tools.FindFridaTool("frida-ps")
	devicesTool := tools.FindFridaTool("frida-ls-devices")
	if !fridaTool.Found || !psTool.Found || !devicesTool.Found {
		return ToolStatus{Name: "frida", Path: fridaTool.Path, Found: false, Source: fridaTool.Source, Error: "缺少 frida、frida-ps 或 frida-ls-devices"}, false
	}
	version := commandVersion(ctx, fridaTool.Path)
	if normalizeVersion(version) != normalizeVersion(expected) {
		return ToolStatus{Name: "frida", Path: fridaTool.Path, Found: false, Source: fridaTool.Source, Version: version, Error: fmt.Sprintf("版本不匹配，期望 %s", expected)}, false
	}
	return ToolStatus{Name: "frida", Path: fridaTool.Path, Found: true, Source: fridaTool.Source, Version: version}, true
}

func commandVersion(ctx context.Context, path string) string {
	output, err := runOutput(ctx, path, "--version")
	if err != nil {
		return ""
	}
	return firstLine(output)
}

func runHidden(ctx context.Context, path string, args ...string) error {
	cmd := processutil.HideWindow(exec.CommandContext(ctx, path, args...))
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			return fmt.Errorf("%w: %s", err, message)
		}
		return err
	}
	return nil
}

func runOutput(ctx context.Context, path string, args ...string) (string, error) {
	cmd := processutil.HideWindow(exec.CommandContext(ctx, path, args...))
	output, err := cmd.CombinedOutput()
	return string(output), err
}

type runtimeMetadata struct {
	Version     string `json:"version"`
	Arch        string `json:"arch"`
	SHA256      string `json:"sha256,omitempty"`
	FileSHA256  string `json:"fileSha256,omitempty"`
	AssetSHA256 string `json:"assetSha256,omitempty"`
	URL         string `json:"url"`
}

func writeMetadata(path string, metadata runtimeMetadata) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path+".json", data, 0o600)
}

func readMetadata(path string) (runtimeMetadata, error) {
	data, err := os.ReadFile(path + ".json")
	if err != nil {
		return runtimeMetadata{}, err
	}
	var metadata runtimeMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return runtimeMetadata{}, err
	}
	return metadata, nil
}

func verifiedServerCache(path string, asset ReleaseAsset, version string, arch string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	if info.IsDir() || info.Size() == 0 {
		return false, nil
	}
	metadata, err := readMetadata(path)
	if err != nil {
		return false, nil
	}
	if normalizeVersion(metadata.Version) != normalizeVersion(version) || strings.TrimSpace(metadata.Arch) != strings.TrimSpace(arch) {
		return false, nil
	}
	actual, err := fileSHA256(path)
	if err != nil {
		return false, err
	}
	fileDigest := metadata.FileSHA256
	if fileDigest == "" {
		fileDigest = metadata.SHA256
	}
	fileDigest = digestValue(fileDigest)
	if !validSHA256(fileDigest) || !strings.EqualFold(actual, fileDigest) {
		return false, nil
	}
	expectedAsset := digestValue(asset.Digest)
	if !validSHA256(expectedAsset) || !validSHA256(metadata.AssetSHA256) || !strings.EqualFold(metadata.AssetSHA256, expectedAsset) {
		return false, nil
	}
	return true, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func findServerAsset(assets []ReleaseAsset, version, arch string) (ReleaseAsset, error) {
	wanted := "frida-server-" + version + "-android-" + arch + ".xz"
	for _, asset := range assets {
		if asset.Name == wanted && isGitHubDownloadURL(asset.BrowserDownloadURL) {
			return asset, nil
		}
	}
	return ReleaseAsset{}, fmt.Errorf("官方 Release 中没有 %s", wanted)
}

func isGitHubDownloadURL(value string) bool {
	return strings.HasPrefix(value, "https://github.com/") || strings.HasPrefix(value, "https://objects.githubusercontent.com/")
}

func normalizeAndroidArch(abi string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(abi))
	switch {
	case strings.Contains(value, "arm64") || strings.Contains(value, "aarch64"):
		return "arm64", nil
	case strings.Contains(value, "armeabi") || value == "arm" || strings.Contains(value, "arm-v7"):
		return "arm", nil
	case strings.Contains(value, "x86_64") || strings.Contains(value, "amd64"):
		return "x86_64", nil
	case value == "x86" || strings.Contains(value, "i686"):
		return "x86", nil
	default:
		return "", fmt.Errorf("不支持的 Android ABI: %s", abi)
	}
}

func normalizeVersion(version string) string {
	version = strings.TrimSpace(strings.TrimPrefix(version, "v"))
	if version == "" || version == "latest" {
		return version
	}
	return version
}

func digestValue(digest string) string {
	if strings.HasPrefix(strings.ToLower(digest), "sha256:") {
		return strings.TrimSpace(digest[len("sha256:"):])
	}
	return strings.TrimSpace(digest)
}

func validSHA256(value string) bool {
	value = digestValue(value)
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func firstLine(value string) string {
	for _, line := range strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n") {
		if value := strings.TrimSpace(line); value != "" {
			return value
		}
	}
	return ""
}

// Keep these helpers in this package so unit tests can validate archive handling
// without making network requests.
func extractZipSafe(destination string, reader *zip.Reader) error {
	for _, file := range reader.File {
		name := filepath.Clean(file.Name)
		if name == "." || filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(filepath.Separator)) || name == ".." {
			return fmt.Errorf("ZIP 包含不安全路径: %s", file.Name)
		}
		target := filepath.Join(destination, name)
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		input, err := file.Open()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeErr := errors.Join(output.Close(), input.Close())
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func sortedAssetNames(assets []ReleaseAsset) []string {
	names := make([]string, 0, len(assets))
	for _, asset := range assets {
		names = append(names, asset.Name)
	}
	sort.Strings(names)
	return names
}
