# Frida GUI Helper
English | [中文](README-CN.md)

A Windows desktop helper for using Frida with Android devices. It wraps common ADB and Frida workflows in a Wails + Vue interface, so users can scan devices, manage `frida-server`, browse processes, inject scripts, import custom scripts, and inspect logs without memorizing long command lines.

> Use this tool only on devices and applications you own or are explicitly authorized to test.

## Features

- Android USB device discovery through ADB.
- App and process list with Attach and Spawn actions.
- `frida-server` push/start/stop helpers for rooted Android devices.
- Local toolchain priority: bundled `tools/` binaries are used before system `PATH`.
- Built-in Frida Script Hub with beginner and advanced templates.
- Import local `.js`, `.mjs`, or `.ts` Frida scripts and run them from the editor.
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

1. Download or build the application.
2. Put optional bundled tools in the `tools/` directory shown below.
3. Start `Frida-GUI-Helper.exe`.
4. Select an Android device on the Devices page.
5. Run `工具链诊断` and `Frida 官方 smoke test`.
6. Start or push `frida-server` from the UI.
7. Open the Processes page and choose an App or process.
8. Open the Scripts page, select a template or import your own script, then run Attach or Spawn.

## Bundled Tools Layout

The app first searches for tools next to the executable and in the current project directory. If not found, it falls back to the system `PATH`.

Recommended layout:

```text
tools/
  platform-tools/
    adb.exe
    AdbWinApi.dll
    AdbWinUsbApi.dll

  frida/
    frida.exe
    frida-ps.exe
    frida-ls-devices.exe

  frida-server/
    android-arm64/
      frida-server
    android-arm/
      frida-server
    android-x86/
      frida-server
    android-x86_64/
      frida-server
```

If `frida-server` is left blank in the UI, the app tries to find the matching file in `tools/frida-server/` based on the device ABI.

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

cd frontend
npm run build
cd ..
```

Run Wails development mode:

```powershell
wails dev
```

Build Windows executable:

```powershell
$env:GOFLAGS="-buildvcs=false"
wails build
```

The packaged executable is generated under `build/bin/`.

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

## License

MIT License. See [LICENSE](LICENSE).
