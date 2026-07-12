# Frida GUI Helper

一个面向 Windows 桌面的 Frida Android 辅助工具。项目使用 Wails + Vue 构建，把常见 ADB 和 Frida 操作封装成图形界面，方便用户完成设备扫描、`frida-server` 管理、进程选择、脚本注入、自定义脚本导入和实时日志查看。

> 请仅在你拥有或已获得明确授权的设备和应用上使用本工具。
> 程序测试设备为MI 6/OnePlus 8T(已root)

## 功能特性

- 通过 ADB 扫描 Android USB 设备。
- 展示 App 和进程列表，支持 Attach 和 Spawn。
- 支持 Root 设备上的 `frida-server` 推送、启动、停止和诊断。
- 本地工具链优先：优先使用 `tools/` 中随软件提供的工具，再回退到系统 `PATH`。
- 内置 Frida Script Hub，包含基础模板和高阶模板。
- 支持导入本地 `.js`、`.mjs`、`.ts` Frida 脚本并在编辑器中运行。
- 实时日志控制台，支持过滤、清空和导出。
- 提供 ADB、Frida CLI、设备 ABI、Root、server 版本、server 进程、官方 `frida-ps -U` smoke test 等诊断操作。

## 内置脚本模板

基础模板：

- 连接探针
- Java Runtime 探针
- SSL Pinning 基础绕过
- Intent 传递监控
- 已加载类枚举

高阶模板：

- 使用 `Memory.scan` 进行内存扫描和补丁
- DEX 内存扫描与 Dump
- 使用 `Stalker` 追踪 Native 执行流
- 使用 `Java.registerClass` 动态创建 Java 类
- 使用 `NativeFunction` 和 `rpc.exports` 进行 Native 调用与 RPC
- 使用 `Interceptor.replace` 和 `Arm64Writer` 替换 Native 函数

所有模板在注入前都可以编辑。高阶模板默认包含占位参数，使用前需要根据目标修改模块名、导出函数名、特征码、返回类型和 patch 字节等内容。

## 环境要求

- Windows amd64
- 通过 USB 连接的 Android 设备
- Android 设备已开启 USB 调试
- 使用 `frida-server` 工作流时，Android 设备通常需要 Root
- 电脑端 Frida CLI 与 Android 端 `frida-server` 版本必须完全一致

## 快速开始

1. 下载或自行构建应用。
2. 按需把 ADB、Frida CLI 和 `frida-server` 放入 `tools/` 目录。
3. 启动 `Frida-GUI-Helper.exe`。
4. 在“设备”页面选择 Android 设备。
5. 运行“工具链诊断”和“Frida 官方 smoke test”。
6. 在界面中启动或推送 `frida-server`。
7. 打开“进程”页面，选择 App 或进程。
8. 打开“脚本”页面，选择内置模板或导入自己的脚本，然后执行 Attach 或 Spawn。

## 本地工具目录

程序会优先在 exe 同目录和当前项目目录中查找 `tools/`。如果找不到，才会使用系统 `PATH` 中的工具。

推荐目录结构：

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

如果 UI 中的 `frida-server` 本地路径留空，程序会根据设备 ABI 自动尝试从 `tools/frida-server/` 中选择匹配文件。

## 开发构建

安装前端依赖：

```powershell
cd frontend
npm install --cache .npm-cache
cd ..
```

运行检查：

```powershell
$env:GOCACHE="$PWD\.gocache"
$env:GOMODCACHE="$PWD\.gomodcache"
$env:GOPATH="$PWD\.gopath"
go test ./...

cd frontend
npm run build
cd ..
```

启动 Wails 开发模式：

```powershell
wails dev
```

构建 Windows 可执行文件：

```powershell
$env:GOFLAGS="-buildvcs=false"
wails build
```

打包后的程序位于：

```text
build/bin/
```

## 常见问题排查

如果 Spawn 或 Attach 失败，请先验证 Frida 基础链路：

1. 运行“工具链诊断”，确认当前使用的是 `bundled` 工具还是系统 `PATH` 工具。
2. 运行“检查 frida-server”，确认 PID 存在。
3. 运行“检查 server 身份”，确认 `frida-server` 以 `root` 身份运行。
4. 运行“检查 server 版本”，确认版本与电脑端 Frida CLI 完全一致。
5. 运行“Frida 官方 smoke test”，确认 `frida-ps -U` 正常。

如果 `frida-ps -U` 失败，脚本注入通常也会失败。请升级电脑端 Frida CLI 和 Android 端 `frida-server` 到官方同一版本后重试。

## fork安全说明

- 不要提交私钥、`.env` 文件、证书、Android 签名文件或本地工具缓存。
- 不要提交个人机器路径、用户名或本地日志。
- ADB、Frida release、`frida-server` 等第三方二进制文件建议通过 GitHub Release assets 发布，不建议直接提交到源码仓库。

## 界面展示
<img width="1258" height="805" alt="8c813534eb054a23ab443da7042553f" src="https://github.com/user-attachments/assets/a0cfc4ad-4dec-44fd-8908-04d04d519678" />
<img width="1250" height="806" alt="bee04719228b2bb46db50054a9b412c" src="https://github.com/user-attachments/assets/62df9a49-58e3-45d0-a050-2e24e714179f" />
<img width="1247" height="793" alt="69cf35dc37d243ec78038b35aade45c" src="https://github.com/user-attachments/assets/f24c2520-02f9-4675-adcc-9b05b6cfed28" />
<img width="1249" height="793" alt="f210239719275f842b0b3ca6e7da430" src="https://github.com/user-attachments/assets/4d767e6f-d874-435c-ad79-9272e79b6b36" />




## 参考资料

- Frida 官方文档：https://frida.re/docs/home/
- Android 配置说明：https://frida.re/docs/android/
- JavaScript API：https://frida.re/docs/javascript-api/
- Frida Releases：https://github.com/frida/frida/releases

## 友情链接

- LinuxDo： https://linux.do/

## 许可证

本项目使用 MIT License。详见 [LICENSE](LICENSE)。
