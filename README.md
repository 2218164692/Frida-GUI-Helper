# Frida GUI Helper
English | [中文](README-CN.md)

A Windows desktop helper for using Frida with Android devices. It wraps common ADB and Frida workflows in a Wails + Vue interface, so users can scan devices, manage `frida-server`, browse processes, inject scripts, import custom scripts, and inspect logs without memorizing long command lines.

> Use this tool only on devices and applications you own or are explicitly authorized to test.

## Features

- Android USB device discovery through ADB.
- App and process list with Attach and Spawn actions.
- `frida-server` push/start/stop helpers for rooted Android devices.
- Isolated Frida toolchain: the desktop CLI, portable Python, and Android `frida-server` are loaded only from the installed `frida/` directory, never from a system Frida installation.
- Built-in Frida Script Hub with beginner and advanced templates.
- Import local `.js`, `.mjs`, or `.ts` Frida scripts and run them from the editor.
- Native templates wait for target modules, including Spawn and delayed `.so` loading.
- CodeShare scripts are loaded for review first and trusted by fingerprint only when run.
- Session exits are synchronized to the UI and removed from the active session list.
- The installer ships the latest official Frida available at build time plus Android servers for arm64, arm, x86_64, and x86. The Devices page validates and pushes the matching same-version server by ABI.
- A Settings page checks application and official Frida updates and controls startup checks, Frida component auto-update, and update notifications.
- Real-time stdout/stderr log console with filtering, clearing, and export.
- Diagnostics for ADB, Frida CLI, device ABI, Root, server version, server process, and official `frida-ps -U` smoke test.

## Built-In Script Templates

Basic templates:

- Connection probe
- Java runtime probe
- SSL Pinning basic bypass
- Intent monitor
- Loaded class enumeration

Advanced templates:

- Memory scanning and patching with `Memory.scan`
- DEX memory scan and dump
- Native execution tracing with `Stalker`
- Runtime Java class creation with `Java.registerClass`
- Native calls and RPC with `NativeFunction` and `rpc.exports`
- Native replacement with `Interceptor.replace` and `Arm64Writer`

All templates are editable before injection. Advanced templates contain placeholders for module names, exported symbols, signatures, return types, and patch bytes.

## Requirements

- Windows amd64
- Android device connected over USB
- USB debugging enabled
- Rooted Android device for `frida-server` workflows
- Matching Frida versions on desktop and Android device

The desktop Frida CLI version and the Android `frida-server` version must match exactly.

## Quick Start

1. Download `Frida-GUI-Helper-v*-windows-amd64-setup.exe` from Releases.
2. Run the installer and choose an installation directory.
3. Start Frida GUI Helper from the Start menu or desktop shortcut.
4. Select an Android device on the Devices page.
5. Run `工具链诊断` and `Frida 官方 smoke test`.
6. Start or push `frida-server` from the UI.
7. Open the Processes page and choose an App or process.
8. Open the Scripts page, select a template or import your own script, then run Attach or Spawn.

The installer contains one locked Frida component set. Its desktop CLI and all four Android servers come from the same official Frida release; GitHub's SHA-256 asset digest is verified when the bundle is built and recorded in the component manifest. The Devices page no longer downloads Frida on first use and never chooses a version from the user's machine, so a matching server can be pushed while offline. ADB may still come from bundled Platform-tools or `PATH`. USB debugging, ADB authorization, and root access are still required on the device.

## Installed Component Layout

Frida resolution is restricted to the managed installation directory:

Recommended layout:

```text
Frida GUI Helper/
  Frida-GUI-Helper.exe
  uninstall.exe
  frida/
    manifest.json
    launcher.py
    frida.exe
    frida-ps.exe
    frida-ls-devices.exe
    python/
    server/
      android-arm64/frida-server
      android-arm/frida-server
      android-x86/frida-server
      android-x86_64/frida-server
```

`manifest.json` is the version source of truth for both desktop and device-side components. A Frida update replaces the CLI and every ABI server as one same-version set.

## Development

Install frontend dependencies:

```powershell
cd frontend
npm install --cache .npm-cache
cd ..
```

Run checks:

```powershell
$env:GOCACHE="$PWD\.gocache"
$env:GOMODCACHE="$PWD\.gomodcache"
$env:GOPATH="$PWD\.gopath"
go test ./...

.\tools\check-version.ps1

cd frontend
npm ci
npm run build
cd ..
```

The repository includes a Windows GitHub Actions workflow for version consistency, frontend builds, Go tests, and `go vet`. The root `VERSION` file is the release source of truth; keep `frontend/package.json` and `wails.json` synchronized when bumping it. CI rejects inconsistent version metadata.

Run Wails development mode:

```powershell
wails dev
```

Prepare the latest Frida payload and build the Windows installer:

```powershell
$env:GOFLAGS="-buildvcs=false"
.\tools\prepare-frida-bundle.ps1 -Version latest
.\tools\build-installer.ps1 -Architecture amd64
```

The build host also needs Wails v2.12 and NSIS. The release artifact is generated as `build/bin/Frida-GUI-Helper-v1.0.8-windows-amd64-setup.exe`.

## Troubleshooting

If Spawn or Attach fails, first verify the Frida base chain:

1. `工具链诊断`: confirm whether tools are loaded from `bundled` or `PATH`.
2. `检查 frida-server`: confirm PID exists.
3. `检查 server 身份`: confirm `frida-server` runs as `root`.
4. `检查 server 版本`: confirm it matches the desktop Frida CLI version.
5. `Frida 官方 smoke test`: confirm `frida-ps -U` works.

If `frida-ps -U` fails, script injection will also fail. Upgrade the desktop Frida CLI and Android `frida-server` to the same official release, then retry.

## Security Notes

- Do not commit private keys, `.env` files, certificates, Android signing keys, or local tool caches.
- Do not commit personal machine paths or user-specific logs.
- Large third-party binaries such as platform-tools and Frida releases should be distributed through release assets if needed.

## References

- Frida documentation: https://frida.re/docs/home/
- Android setup: https://frida.re/docs/android/
- JavaScript API: https://frida.re/docs/javascript-api/
- Frida releases: https://github.com/frida/frida/releases
- LinuxDo：https://linux.do
- 52pj：https://www.52pojie.cn/

## License

MIT License. See [LICENSE](LICENSE).
