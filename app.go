package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"frida-gui-helper/internal/adb"
	"frida-gui-helper/internal/codeshare"
	"frida-gui-helper/internal/diagnostics"
	"frida-gui-helper/internal/frida"
	"frida-gui-helper/internal/logstream"
	"frida-gui-helper/internal/operations"
	"frida-gui-helper/internal/processutil"
	runtimeenv "frida-gui-helper/internal/runtime"
	"frida-gui-helper/internal/scripts"
	"frida-gui-helper/internal/scriptstore"
	appsettings "frida-gui-helper/internal/settings"
	"frida-gui-helper/internal/toolchain"
	"frida-gui-helper/internal/updater"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx              context.Context
	logs             *logstream.Stream
	adb              *adb.Runner
	frida            *frida.Runner
	codeShare        *codeshare.Client
	scripts          *scriptstore.Store
	tools            *toolchain.Resolver
	runtime          *runtimeenv.Manager
	settings         *appsettings.Store
	updates          *updater.Checker
	updateMu         sync.RWMutex
	fridaComponentMu sync.RWMutex
	appUpdate        updater.ComponentStatus
	fridaUpdate      updater.ComponentStatus
	cancel           context.CancelFunc
}

type UpdateOverview struct {
	Application      updater.ComponentStatus `json:"application"`
	Frida            updater.ComponentStatus `json:"frida"`
	InstallDirectory string                  `json:"installDirectory"`
	FridaDirectory   string                  `json:"fridaDirectory"`
}

type SystemStatus struct {
	ADB         adb.ToolStatus   `json:"adb"`
	Frida       frida.ToolStatus `json:"frida"`
	Python      ToolStatus       `json:"python"`
	GeneratedAt string           `json:"generatedAt"`
}

type ToolStatus struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Found   bool   `json:"found"`
	Source  string `json:"source"`
	Version string `json:"version"`
	Error   string `json:"error"`
}

type FridaServerRequest struct {
	DeviceSerial string `json:"deviceSerial"`
	LocalPath    string `json:"localPath"`
	RemotePath   string `json:"remotePath"`
	ForceRestart bool   `json:"forceRestart"`
}

type FridaRuntimeRequest struct {
	DeviceSerial string `json:"deviceSerial"`
	Version      string `json:"version"`
}

type RunScriptRequest struct {
	DeviceSerial string `json:"deviceSerial"`
	Mode         string `json:"mode"`
	TargetKind   string `json:"targetKind"`
	Target       string `json:"target"`
	ScriptName   string `json:"scriptName"`
	ScriptSource string `json:"scriptSource"`
}

type RunOperationRequest struct {
	ID           string `json:"id"`
	DeviceSerial string `json:"deviceSerial"`
}

type ImportedScript struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Source string `json:"source"`
}

type SaveLocalScriptRequest struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Favorite    bool     `json:"favorite"`
	Source      string   `json:"source"`
	Origin      string   `json:"origin"`
}

func NewApp() *App {
	app := &App{}
	app.tools = toolchain.NewResolver()
	app.logs = logstream.New(1000, nil)
	app.adb = adb.NewRunner(app.addLog, app.tools)
	app.frida = frida.NewRunner(app.addLog, app.tools)
	app.runtime = runtimeenv.NewManager(app.tools)
	settingsStore, err := appsettings.New()
	if err != nil {
		settingsStore = appsettings.NewAt(filepath.Join(os.TempDir(), "Frida-GUI-Helper", "settings.json"))
	}
	app.settings = settingsStore
	app.updates = updater.NewChecker()
	app.appUpdate = updater.ComponentStatus{
		Component:      updater.ComponentApplication,
		Repository:     updater.ApplicationRepository,
		CurrentVersion: applicationVersion(),
	}
	app.fridaUpdate = updater.ComponentStatus{
		Component:      updater.ComponentFrida,
		Repository:     updater.FridaRepository,
		CurrentVersion: app.bundledFridaVersion(),
	}
	app.codeShare = codeshare.NewClient()
	app.scripts = scriptstore.New()
	return app
}

func (a *App) startup(ctx context.Context) {
	appCtx, cancel := context.WithCancel(ctx)
	a.ctx = appCtx
	a.cancel = cancel
	a.logs.SetEmitter(func(entry logstream.Entry) {
		wailsRuntime.EventsEmit(ctx, "log:new", entry)
	})
	a.frida.SetSessionEmitter(func(info frida.SessionInfo) {
		wailsRuntime.EventsEmit(ctx, "session:updated", info)
	})
	a.runtime.SetProgressEmitter(func(progress runtimeenv.Progress) {
		wailsRuntime.EventsEmit(ctx, "runtime:progress", progress)
	})
	a.addLog("info", "system", "应用已启动")
	go a.runAutomaticUpdateChecks(appCtx)
}

func (a *App) shutdown(ctx context.Context) {
	a.frida.SetSessionEmitter(nil)
	a.runtime.SetProgressEmitter(nil)
	a.frida.StopAll()
	if a.cancel != nil {
		a.cancel()
	}
}

func (a *App) GetSystemStatus() SystemStatus {
	ctx, cancel := a.withTimeout(8 * time.Second)
	defer cancel()

	bundled := a.runtime.BundledStatus(ctx, "")
	return SystemStatus{
		ADB:         a.adb.Status(ctx),
		Frida:       a.frida.Status(ctx),
		Python:      ToolStatus(bundled.Python),
		GeneratedAt: time.Now().Format(time.RFC3339),
	}
}

func (a *App) ListDevices() ([]adb.Device, error) {
	ctx, cancel := a.withTimeout(12 * time.Second)
	defer cancel()
	return a.adb.Devices(ctx)
}

func (a *App) ListApps(deviceSerial string, includeSystem bool) ([]adb.AndroidApp, error) {
	if strings.TrimSpace(deviceSerial) == "" {
		return nil, errors.New("请先选择 Android 设备")
	}
	ctx, cancel := a.withTimeout(25 * time.Second)
	defer cancel()
	return a.adb.ListPackages(ctx, deviceSerial, includeSystem)
}

func (a *App) ListProcesses(deviceSerial string) ([]adb.Process, error) {
	if strings.TrimSpace(deviceSerial) == "" {
		return nil, errors.New("请先选择 Android 设备")
	}
	ctx, cancel := a.withTimeout(20 * time.Second)
	defer cancel()
	return a.adb.ListProcesses(ctx, deviceSerial)
}

func (a *App) StartFridaServer(req FridaServerRequest) error {
	if strings.TrimSpace(req.DeviceSerial) == "" {
		return errors.New("请先选择 Android 设备")
	}
	if !a.fridaComponentMu.TryRLock() {
		return errors.New("Frida 组件正在更新，请稍后再启动 frida-server")
	}
	defer a.fridaComponentMu.RUnlock()
	ctx, cancel := a.withTimeout(60 * time.Second)
	defer cancel()
	var expectedVersion string
	req.LocalPath, expectedVersion = a.findBundledFridaServer(ctx, req.DeviceSerial)
	if strings.TrimSpace(req.LocalPath) == "" {
		return errors.New("没有找到与设备 ABI 匹配的安装包内置 frida-server")
	}
	return a.adb.StartFridaServer(ctx, adb.FridaServerRequest{
		DeviceSerial:    req.DeviceSerial,
		LocalPath:       req.LocalPath,
		RemotePath:      req.RemotePath,
		ForceRestart:    true,
		ExpectedVersion: expectedVersion,
	})
}

func (a *App) GetFridaRuntimeStatus() runtimeenv.Status {
	ctx, cancel := a.withTimeout(8 * time.Second)
	defer cancel()
	return a.runtime.BundledStatus(ctx, "")
}

func (a *App) PrepareFridaRuntime(req FridaRuntimeRequest) (runtimeenv.Status, error) {
	serial := strings.TrimSpace(req.DeviceSerial)
	if serial == "" {
		return runtimeenv.Status{}, errors.New("请先选择 Android 设备")
	}
	if !a.fridaComponentMu.TryRLock() {
		return runtimeenv.Status{}, errors.New("Frida 组件正在更新，请稍后再配置设备")
	}
	defer a.fridaComponentMu.RUnlock()
	ctx, cancel := a.withTimeout(8 * time.Minute)
	defer cancel()
	if _, err := a.runtime.EnsureADB(ctx); err != nil {
		a.addLog("error", "runtime", err.Error())
		return runtimeenv.Status{}, err
	}
	abi, err := a.adb.ShellQuiet(ctx, serial, "getprop", "ro.product.cpu.abilist")
	if err != nil || strings.TrimSpace(abi) == "" {
		abi, err = a.adb.ShellQuiet(ctx, serial, "getprop", "ro.product.cpu.abi")
	}
	if err != nil || strings.TrimSpace(abi) == "" {
		return runtimeenv.Status{}, fmt.Errorf("读取 Android ABI 失败: %w", err)
	}
	a.addLog("info", "runtime", fmt.Sprintf("校验安装包内置 Frida，设备 ABI: %s", strings.TrimSpace(abi)))
	status, prepareErr := a.runtime.PrepareBundled(ctx, strings.TrimSpace(abi))
	if prepareErr != nil {
		a.addLog("error", "runtime", prepareErr.Error())
		return status, prepareErr
	}
	if err := a.adb.StartFridaServer(ctx, adb.FridaServerRequest{
		DeviceSerial:    serial,
		LocalPath:       status.Server.Path,
		RemotePath:      adb.DefaultFridaServerRemotePath,
		ForceRestart:    true,
		ExpectedVersion: status.LatestVersion,
	}); err != nil {
		a.addLog("error", "runtime", "Frida server 推送或启动失败: "+err.Error())
		return status, err
	}
	status.Server.Source = "managed + device"
	a.addLog("info", "runtime", fmt.Sprintf("内置 Frida %s 已推送并启动: %s", status.LatestVersion, status.Server.Path))
	return status, nil
}

func (a *App) GetSettings() (appsettings.Settings, error) {
	if a.settings == nil {
		return appsettings.Settings{}, errors.New("设置存储未初始化")
	}
	return a.settings.Load()
}

func (a *App) SaveSettings(value appsettings.Settings) (appsettings.Settings, error) {
	if a.settings == nil {
		return appsettings.Settings{}, errors.New("设置存储未初始化")
	}
	if err := a.settings.Save(value); err != nil {
		a.addLog("error", "settings", "保存设置失败: "+err.Error())
		return appsettings.Settings{}, err
	}
	a.addLog("info", "settings", "更新设置已保存")
	return value, nil
}

func (a *App) GetUpdateOverview() UpdateOverview {
	a.updateMu.RLock()
	application := a.appUpdate
	fridaStatus := a.fridaUpdate
	a.updateMu.RUnlock()
	application.CurrentVersion = applicationVersion()
	fridaStatus.CurrentVersion = a.bundledFridaVersion()
	return UpdateOverview{
		Application:      application,
		Frida:            fridaStatus,
		InstallDirectory: installationDirectory(),
		FridaDirectory:   a.runtime.BundleRoot(),
	}
}

func (a *App) CheckApplicationUpdate() (updater.ComponentStatus, error) {
	ctx, cancel := a.withTimeout(20 * time.Second)
	defer cancel()
	status, err := a.updates.CheckApplication(ctx, applicationVersion())
	a.updateMu.Lock()
	a.appUpdate = status
	a.updateMu.Unlock()
	a.emitUpdateOverview()
	if err != nil {
		a.addLog("warn", "update", "软件更新检查失败: "+err.Error())
		return status, err
	}
	if status.UpdateAvailable {
		a.addLog("info", "update", fmt.Sprintf("发现 Frida GUI Helper %s", status.LatestVersion))
	}
	return status, nil
}

func (a *App) CheckFridaUpdate() (updater.ComponentStatus, error) {
	ctx, cancel := a.withTimeout(20 * time.Second)
	defer cancel()
	status, err := a.updates.CheckFrida(ctx, a.bundledFridaVersion())
	a.updateMu.Lock()
	a.fridaUpdate = status
	a.updateMu.Unlock()
	a.emitUpdateOverview()
	if err != nil {
		a.addLog("warn", "update", "Frida 更新检查失败: "+err.Error())
		return status, err
	}
	if status.UpdateAvailable {
		a.addLog("info", "update", fmt.Sprintf("发现官方 Frida %s", status.LatestVersion))
	}
	return status, nil
}

func (a *App) UpdateFridaComponent() (runtimeenv.Status, error) {
	if len(a.frida.ListSessions()) > 0 {
		return runtimeenv.Status{}, errors.New("请先停止全部 Frida 脚本会话，再更新组件")
	}
	latest, err := a.CheckFridaUpdate()
	if err != nil {
		return runtimeenv.Status{}, err
	}
	if strings.TrimSpace(latest.LatestVersion) == "" {
		return runtimeenv.Status{}, errors.New("没有获取到可用的 Frida 版本")
	}
	if !a.fridaComponentMu.TryLock() {
		return runtimeenv.Status{}, errors.New("Frida 组件正在使用或已有更新任务正在运行")
	}
	defer a.fridaComponentMu.Unlock()
	if len(a.frida.ListSessions()) > 0 {
		return runtimeenv.Status{}, errors.New("请先停止全部 Frida 脚本会话，再更新组件")
	}
	ctx, cancel := a.withTimeout(20 * time.Minute)
	defer cancel()
	a.addLog("info", "update", "开始更新内置 Frida "+latest.LatestVersion)
	manifest, err := a.runtime.InstallBundle(ctx, latest.LatestVersion)
	if err != nil {
		a.addLog("error", "update", "Frida 组件更新失败: "+err.Error())
		return a.runtime.BundledStatus(ctx, ""), err
	}
	status := a.runtime.BundledStatus(ctx, "")
	if !status.Host.Found || !status.Server.Found {
		err := errors.New("Frida 组件下载完成，但完整性检查未通过")
		a.addLog("error", "update", err.Error())
		return status, err
	}
	latest.CurrentVersion = manifest.Version
	latest.LatestVersion = manifest.Version
	latest.UpdateAvailable = false
	latest.Error = ""
	a.updateMu.Lock()
	a.fridaUpdate = latest
	a.updateMu.Unlock()
	a.emitUpdateOverview()
	a.addLog("info", "update", fmt.Sprintf("内置 Frida 已更新到 %s；下次启动守护时会推送同版本 server", manifest.Version))
	return status, nil
}

func (a *App) runAutomaticUpdateChecks(ctx context.Context) {
	preferences, err := a.GetSettings()
	if err != nil {
		a.addLog("warn", "settings", "读取更新设置失败: "+err.Error())
		return
	}
	var notices []string
	if preferences.AutoCheckAppUpdates && ctx.Err() == nil {
		status, _ := a.CheckApplicationUpdate()
		if status.UpdateAvailable {
			notices = append(notices, "Frida GUI Helper "+status.LatestVersion+" 已发布")
		}
	}
	var fridaStatus updater.ComponentStatus
	if (preferences.AutoCheckFridaUpdates || preferences.AutoUpdateFrida) && ctx.Err() == nil {
		fridaStatus, _ = a.CheckFridaUpdate()
		if fridaStatus.UpdateAvailable {
			notices = append(notices, "官方 Frida "+fridaStatus.LatestVersion+" 可用")
		}
	}
	if preferences.NotifyWhenAvailable && len(notices) > 0 && a.ctx != nil {
		wailsRuntime.EventsEmit(a.ctx, "update:notification", strings.Join(notices, "；"))
	}
	if preferences.AutoUpdateFrida && fridaStatus.UpdateAvailable && ctx.Err() == nil {
		_, _ = a.UpdateFridaComponent()
	}
}

func (a *App) emitUpdateOverview() {
	if a.ctx != nil {
		wailsRuntime.EventsEmit(a.ctx, "update:status", a.GetUpdateOverview())
	}
}

func (a *App) bundledFridaVersion() string {
	manifest, err := a.runtime.ReadBundleManifest()
	if err != nil {
		return ""
	}
	return manifest.Version
}

func applicationVersion() string {
	version := strings.TrimSpace(strings.TrimPrefix(versionText, "v"))
	if version == "" {
		return "0.0.0"
	}
	return version
}

func installationDirectory() string {
	if executable, err := os.Executable(); err == nil {
		return filepath.Dir(executable)
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return ""
}

func (a *App) ListScripts() []scripts.Template {
	return scripts.List()
}

func (a *App) ListLocalScripts() ([]scriptstore.Script, error) {
	items, err := a.scripts.List()
	if err != nil {
		a.addLog("error", "scripts", err.Error())
		return nil, err
	}
	return items, nil
}

func (a *App) SaveLocalScript(req SaveLocalScriptRequest) (scriptstore.Script, error) {
	saved, err := a.scripts.Save(scriptstore.SaveRequest{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
		Tags:        req.Tags,
		Favorite:    req.Favorite,
		Source:      req.Source,
		Origin:      req.Origin,
	})
	if err != nil {
		a.addLog("error", "scripts", err.Error())
		return scriptstore.Script{}, err
	}
	a.addLog("info", "scripts", fmt.Sprintf("已保存本地脚本: %s", saved.Name))
	return saved, nil
}

func (a *App) DeleteLocalScript(id string) error {
	if err := a.scripts.Delete(id); err != nil {
		a.addLog("error", "scripts", err.Error())
		return err
	}
	a.addLog("info", "scripts", "已删除本地脚本")
	return nil
}

func (a *App) RecordLocalScriptRun(id string) error {
	if err := a.scripts.RecordRun(id); err != nil {
		a.addLog("warn", "scripts", err.Error())
		return err
	}
	return nil
}

func (a *App) SearchCodeShare(query string, page int) (codeshare.SearchResult, error) {
	ctx, cancel := a.withTimeout(20 * time.Second)
	defer cancel()

	result, err := a.codeShare.Search(ctx, query, page)
	if err != nil {
		a.addLog("error", "codeshare", err.Error())
		return codeshare.SearchResult{}, err
	}
	if result.Warning != "" {
		a.addLog("warn", "codeshare", result.Warning)
	} else {
		a.addLog("info", "codeshare", fmt.Sprintf("已加载 %d 个项目（第 %d/%d 页，来源: %s）", len(result.Items), result.Page, result.TotalPages, result.Source))
	}
	return result, nil
}

func (a *App) GetCodeShareProject(projectRef string) (codeshare.Project, error) {
	ctx, cancel := a.withTimeout(20 * time.Second)
	defer cancel()

	project, err := a.codeShare.GetProject(ctx, projectRef)
	if err != nil {
		a.addLog("error", "codeshare", err.Error())
		return codeshare.Project{}, err
	}
	if project.Warning != "" {
		a.addLog("warn", "codeshare", project.Warning)
	}
	a.addLog("info", "codeshare", fmt.Sprintf("已加载 @%s，来源: %s，SHA-256: %s", project.Ref, project.Origin, project.Fingerprint))
	return project, nil
}

func (a *App) TrustCodeShareProject(projectRef string, fingerprint string) error {
	if err := a.codeShare.Trust(projectRef, fingerprint); err != nil {
		a.addLog("error", "codeshare", err.Error())
		return err
	}
	a.addLog("info", "codeshare", fmt.Sprintf("已信任 @%s，SHA-256: %s", strings.TrimPrefix(strings.TrimSpace(projectRef), "@"), fingerprint))
	return nil
}

func (a *App) ListOperations() []operations.Template {
	return operations.List()
}

func (a *App) ImportScriptFile() (ImportedScript, error) {
	path, err := wailsRuntime.OpenFileDialog(a.baseContext(), wailsRuntime.OpenDialogOptions{
		Title: "选择 Frida 脚本",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "Frida JavaScript (*.js;*.mjs;*.ts)", Pattern: "*.js;*.mjs;*.ts"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return ImportedScript{}, err
	}
	if strings.TrimSpace(path) == "" {
		return ImportedScript{}, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return ImportedScript{}, err
	}
	if info.Size() > 2*1024*1024 {
		return ImportedScript{}, fmt.Errorf("脚本文件过大: %d bytes，当前限制为 2MB", info.Size())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return ImportedScript{}, err
	}
	return ImportedScript{
		Name:   filepath.Base(path),
		Path:   path,
		Source: string(data),
	}, nil
}

func (a *App) RunOperation(req RunOperationRequest) error {
	id := strings.TrimSpace(req.ID)
	serial := strings.TrimSpace(req.DeviceSerial)
	if requiresDevice(id) && serial == "" {
		return errors.New("请先选择 Android 设备")
	}

	ctx, cancel := a.withTimeout(25 * time.Second)
	defer cancel()

	a.addLog("info", "operation", "开始执行: "+id)
	switch id {
	case "device-abi":
		out, err := a.adb.Shell(ctx, serial, "getprop", "ro.product.cpu.abi")
		a.logCommandResult("adb", "CPU ABI", out, err)
		return err
	case "android-version":
		release, err := a.adb.Shell(ctx, serial, "getprop", "ro.build.version.release")
		a.logCommandResult("adb", "Android release", release, err)
		if err != nil {
			return err
		}
		sdk, err := a.adb.Shell(ctx, serial, "getprop", "ro.build.version.sdk")
		a.logCommandResult("adb", "Android SDK", sdk, err)
		return err
	case "root-check":
		return a.checkRoot(ctx, serial)
	case "toolchain-report":
		return a.toolchainReport(ctx, serial)
	case "frida-server-pid":
		out, err := a.adb.FridaServerPID(ctx, serial)
		if strings.TrimSpace(out) == "" {
			out = "未检测到 frida-server 进程"
		}
		a.logCommandResult("frida-server", "PID", out, err)
		return err
	case "frida-server-user":
		out, err := a.adb.ShellCommandQuiet(ctx, serial, "ps -A | grep frida-server")
		if strings.TrimSpace(out) == "" {
			out = "未检测到 frida-server 进程"
			err = nil
		}
		a.logCommandResult("frida-server", "Process", out, err)
		return err
	case "frida-server-path":
		remotePath, err := a.adb.FindFridaServerBinary(ctx, serial, "")
		a.logCommandResult("frida-server", "Resolved path", remotePath, err)
		return err
	case "frida-server-version":
		remotePath, version, err := a.adb.FridaServerVersion(ctx, serial, "")
		out := ""
		if remotePath != "" {
			out = "Path: " + remotePath
		}
		if version != "" {
			out += "\nVersion: " + version
		}
		a.logCommandResult("frida-server", "Binary", out, err)
		return err
	case "frida-compat-report":
		return a.fridaCompatibilityReport(ctx, serial)
	case "frida-server-start":
		if !a.fridaComponentMu.TryRLock() {
			return errors.New("Frida 组件正在更新，请稍后再启动 frida-server")
		}
		defer a.fridaComponentMu.RUnlock()
		localPath, expectedVersion := a.findBundledFridaServer(ctx, serial)
		err := a.adb.StartFridaServer(ctx, adb.FridaServerRequest{
			DeviceSerial:    serial,
			LocalPath:       localPath,
			RemotePath:      adb.DefaultFridaServerRemotePath,
			ForceRestart:    true,
			ExpectedVersion: expectedVersion,
		})
		if err != nil {
			a.logCommandResult("frida-server", "Start", "", err)
			return err
		}
		time.Sleep(800 * time.Millisecond)
		pid, _ := a.adb.FridaServerPID(ctx, serial)
		if strings.TrimSpace(pid) == "" {
			err := errors.New("启动命令已发送，但未检测到 frida-server 进程；请检查 Root 授权和二进制架构")
			a.logCommandResult("frida-server", "Start", "", err)
			return err
		}
		a.logCommandResult("frida-server", "Start", "frida-server 已启动: "+pid, nil)
		return nil
	case "frida-server-stop":
		out, err := a.adb.StopFridaServer(ctx, serial)
		a.logCommandResult("frida-server", "Stop", out, err)
		return err
	case "frida-server-log":
		out, err := a.adb.ShellQuiet(ctx, serial, "tail", "-n", "200", adb.FridaServerLogPath)
		if strings.TrimSpace(out) == "" {
			out = "没有读取到 server 日志。请停止后重新点击“启动 frida-server”，新版会写入 /data/local/tmp/frida-server.log。"
		}
		a.logCommandResult("frida-server", "Log", out, err)
		return err
	case "frida-logcat":
		out, err := a.adb.ShellCommandQuiet(ctx, serial, "logcat -d -t 400 | grep -i -E 'frida|gum|ptrace|avc|denied|crash|debugger|zygote'")
		if strings.TrimSpace(out) == "" {
			out = "最近 logcat 未匹配到 frida/gum/ptrace/avc/crash 等关键字"
			err = nil
		}
		a.logCommandResult("logcat", "Frida related", out, err)
		return err
	case "adb-forward-frida":
		out1, err := a.adb.Forward(ctx, serial, "tcp:27042", "tcp:27042")
		a.logCommandResult("adb", "forward 27042", out1, err)
		if err != nil {
			return err
		}
		out2, err := a.adb.Forward(ctx, serial, "tcp:27043", "tcp:27043")
		a.logCommandResult("adb", "forward 27043", out2, err)
		return err
	case "frida-devices":
		return a.runCLI(ctx, "frida-ls-devices")
	case "frida-smoke-usb":
		return a.runCLI(ctx, "frida-ps", "-U")
	case "frida-processes":
		return a.runCLI(ctx, "frida-ps", "-D", serial)
	case "frida-apps":
		return a.runCLI(ctx, "frida-ps", "-D", serial, "-a")
	default:
		return fmt.Errorf("未知操作: %s", id)
	}
}

func (a *App) RunScript(req RunScriptRequest) (frida.SessionInfo, error) {
	if strings.TrimSpace(req.DeviceSerial) == "" {
		return frida.SessionInfo{}, errors.New("请先选择 Android 设备")
	}
	if strings.TrimSpace(req.Target) == "" {
		return frida.SessionInfo{}, errors.New("请填写目标进程、PID 或包名")
	}
	if strings.TrimSpace(req.ScriptSource) == "" {
		return frida.SessionInfo{}, errors.New("脚本内容不能为空")
	}
	if !a.fridaComponentMu.TryRLock() {
		return frida.SessionInfo{}, errors.New("Frida 组件正在更新，请稍后再运行脚本")
	}
	defer a.fridaComponentMu.RUnlock()

	return a.frida.RunScript(a.baseContext(), frida.RunRequest{
		DeviceSerial: req.DeviceSerial,
		Mode:         req.Mode,
		TargetKind:   req.TargetKind,
		Target:       req.Target,
		ScriptName:   req.ScriptName,
		ScriptSource: req.ScriptSource,
	})
}

func (a *App) StopSession(sessionID string) error {
	return a.frida.StopSession(sessionID)
}

func (a *App) ListSessions() []frida.SessionInfo {
	return a.frida.ListSessions()
}

func (a *App) GetLogs() []logstream.Entry {
	return a.logs.Entries()
}

func (a *App) ClearLogs() {
	a.logs.Clear()
	a.addLog("info", "system", "日志已清空")
}

func (a *App) withTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(a.baseContext(), timeout)
}

func (a *App) baseContext() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

func (a *App) addLog(level string, source string, message string) {
	if finding, ok := diagnostics.Classify(source, message); ok {
		if level == "info" {
			level = "error"
		}
		a.logs.AddWithDiagnostic(logstream.Level(level), source, message, &finding)
		return
	}
	a.logs.Add(logstream.Level(level), source, message)
}

func (a *App) logCommandResult(source string, title string, output string, err error) {
	level := "info"
	message := strings.TrimSpace(output)
	if err != nil {
		level = "error"
		message = err.Error()
		if strings.TrimSpace(output) != "" {
			message += "\n" + strings.TrimSpace(output)
		}
	} else if message == "" {
		message = "完成"
	}
	a.addLog(level, source, title+": "+message)
}

func (a *App) checkRoot(ctx context.Context, serial string) error {
	attempts := []struct {
		label   string
		command string
	}{
		{"su -c", "su -c 'id'"},
		{"su 0", "su 0 id"},
	}

	var lastErr error
	for _, attempt := range attempts {
		out, err := a.adb.ShellCommandQuiet(ctx, serial, attempt.command)
		if err == nil && strings.Contains(out, "uid=0") {
			a.logCommandResult("adb", "Root check", attempt.label+": "+strings.TrimSpace(out), nil)
			return nil
		}
		if err != nil {
			lastErr = err
			a.addLog("warn", "adb", "Root check "+attempt.label+" 失败: "+err.Error())
			continue
		}
		lastErr = fmt.Errorf("%s 未返回 uid=0: %s", attempt.label, strings.TrimSpace(out))
	}

	shellID, _ := a.adb.ShellCommandQuiet(ctx, serial, "id")
	if strings.TrimSpace(shellID) != "" {
		a.addLog("info", "adb", "当前 adb shell 身份: "+strings.TrimSpace(shellID))
	}
	if lastErr == nil {
		lastErr = errors.New("未获得 root shell")
	}
	a.logCommandResult("adb", "Root check", "设备可能已 root，但 ADB shell 没有被授予 root 权限。请在 Magisk/KernelSU/APatch 中允许 Shell/ADB shell 的超级用户权限。", lastErr)
	return lastErr
}

func (a *App) fridaCompatibilityReport(ctx context.Context, serial string) error {
	a.addLog("info", "diagnostic", "Frida 兼容性诊断开始")

	status := a.frida.Status(ctx)
	if status.Found {
		a.addLog("info", "diagnostic", "Frida CLI: "+status.Version)
	} else {
		a.addLog("error", "diagnostic", "Frida CLI 未找到: "+status.Error)
	}

	checks := []struct {
		title   string
		command string
	}{
		{"Android release", "getprop ro.build.version.release"},
		{"Android SDK", "getprop ro.build.version.sdk"},
		{"ABI", "getprop ro.product.cpu.abilist"},
		{"SELinux", "getenforce"},
		{"server process", "ps -A | grep frida-server"},
	}

	for _, check := range checks {
		out, err := a.adb.ShellCommandQuiet(ctx, serial, check.command)
		if strings.TrimSpace(out) == "" && check.title == "server process" {
			out = "未检测到 frida-server 进程"
			err = nil
		}
		a.logCommandResult("diagnostic", check.title, out, err)
	}

	serverPath, serverVersion, serverErr := a.adb.FridaServerVersion(ctx, serial, "")
	serverInfo := ""
	if serverPath != "" {
		serverInfo = "Path: " + serverPath
	}
	if serverVersion != "" {
		serverInfo += "\nVersion: " + serverVersion
	}
	a.logCommandResult("diagnostic", "server binary", serverInfo, serverErr)

	err := a.runCLI(ctx, "frida-ps", "-U")
	if err != nil {
		a.addLog("error", "diagnostic", "官方 smoke test 未通过。只要 frida-ps -U 失败，Spawn/Attach 和脚本注入都会失败。Android 15 设备建议升级 Frida CLI 与 frida-server 到官方最新同版本后重试。")
		return err
	}
	a.addLog("info", "diagnostic", "官方 smoke test 通过")
	return nil
}

func (a *App) runCLI(ctx context.Context, name string, args ...string) error {
	tool := a.tools.FindExecutable(name)
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), "frida") {
		tool = a.tools.FindFridaTool(name)
	}
	if !tool.Found {
		err := fmt.Errorf("%s not found: %s", name, tool.Error)
		a.logCommandResult(name, name+" "+strings.Join(args, " "), "", err)
		return err
	}
	cmd := processutil.HideWindow(exec.CommandContext(ctx, tool.Path, args...))
	output, err := cmd.CombinedOutput()
	a.logCommandResult(name, name+"["+tool.Source+"] "+strings.Join(args, " "), string(output), err)
	return err
}

func (a *App) toolchainReport(ctx context.Context, serial string) error {
	a.addLog("info", "toolchain", "工具查找目录: "+strings.Join(a.tools.BaseDirs(), " | "))
	for _, name := range []string{"adb", "frida", "frida-ps", "frida-ls-devices"} {
		tool := a.tools.FindExecutable(name)
		if strings.HasPrefix(name, "frida") {
			tool = a.tools.FindFridaTool(name)
		}
		if !tool.Found {
			a.addLog("error", "toolchain", name+": "+tool.Error)
			continue
		}
		a.addLog("info", "toolchain", fmt.Sprintf("%s: %s (%s)", name, tool.Path, tool.Source))
	}
	if serial != "" {
		abi, _ := a.adb.ShellCommandQuiet(ctx, serial, "getprop ro.product.cpu.abi")
		status := a.runtime.BundledStatus(ctx, strings.TrimSpace(abi))
		if status.Server.Found {
			a.addLog("info", "toolchain", fmt.Sprintf("frida-server[%s/%s]: %s", status.LatestVersion, strings.TrimSpace(abi), status.Server.Path))
		} else {
			a.addLog("warn", "toolchain", status.Server.Error)
		}
	}
	return nil
}

func (a *App) findBundledFridaServer(ctx context.Context, serial string) (string, string) {
	abi, _ := a.adb.ShellCommandQuiet(ctx, serial, "getprop ro.product.cpu.abi")
	status := a.runtime.BundledStatus(ctx, strings.TrimSpace(abi))
	if !status.Server.Found {
		a.addLog("warn", "toolchain", status.Server.Error)
		return "", status.LatestVersion
	}
	a.addLog("info", "toolchain", fmt.Sprintf("使用内置 Frida %s server: %s", status.LatestVersion, status.Server.Path))
	return status.Server.Path, status.LatestVersion
}

func requiresDevice(id string) bool {
	for _, item := range operations.List() {
		if item.ID == id {
			return item.RequiresDevice
		}
	}
	return true
}
