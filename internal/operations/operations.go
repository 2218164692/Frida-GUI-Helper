package operations

type Template struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Category       string `json:"category"`
	Description    string `json:"description"`
	RequiresDevice bool   `json:"requiresDevice"`
}

func List() []Template {
	return []Template{
		{
			ID:             "device-abi",
			Name:           "读取设备架构",
			Category:       "设备诊断",
			Description:    "执行 getprop ro.product.cpu.abi，用于选择 arm64/arm/x86 的 frida-server。",
			RequiresDevice: true,
		},
		{
			ID:             "android-version",
			Name:           "读取 Android 版本",
			Category:       "设备诊断",
			Description:    "读取系统版本和 SDK，定位兼容性问题。",
			RequiresDevice: true,
		},
		{
			ID:             "root-check",
			Name:           "检测 Root 权限",
			Category:       "设备诊断",
			Description:    "执行 su -c id，确认 frida-server 是否能以 root 启动。",
			RequiresDevice: true,
		},
		{
			ID:             "toolchain-report",
			Name:           "工具链诊断",
			Category:       "设备诊断",
			Description:    "显示当前使用的 adb、frida、frida-ps 和内置 frida-server 路径。",
			RequiresDevice: false,
		},
		{
			ID:             "frida-server-pid",
			Name:           "检查 frida-server",
			Category:       "Frida-server",
			Description:    "执行 pidof frida-server，确认设备端守护进程是否运行。",
			RequiresDevice: true,
		},
		{
			ID:             "frida-server-user",
			Name:           "检查 server 身份",
			Category:       "Frida-server",
			Description:    "查看 frida-server 进程 USER，注入普通应用通常需要 root。",
			RequiresDevice: true,
		},
		{
			ID:             "frida-server-path",
			Name:           "定位 server 文件",
			Category:       "Frida-server",
			Description:    "识别设备端实际的 frida-server 文件，兼容目标路径为目录和旧版推送文件名。",
			RequiresDevice: true,
		},
		{
			ID:             "frida-server-version",
			Name:           "检查 server 版本",
			Category:       "Frida-server",
			Description:    "自动定位设备端二进制并执行 --version，确认与 Frida CLI 完全一致。",
			RequiresDevice: true,
		},
		{
			ID:             "frida-compat-report",
			Name:           "兼容性诊断",
			Category:       "Frida-server",
			Description:    "汇总 Android 版本、ABI、SELinux、CLI/server 版本和 smoke test 结果。",
			RequiresDevice: true,
		},
		{
			ID:             "frida-server-start",
			Name:           "启动 frida-server",
			Category:       "Frida-server",
			Description:    "自动解析文件或目录路径，验证二进制后使用 root 权限启动。",
			RequiresDevice: true,
		},
		{
			ID:             "frida-server-stop",
			Name:           "停止 frida-server",
			Category:       "Frida-server",
			Description:    "执行 pkill -f frida-server，用于重启或更换版本。",
			RequiresDevice: true,
		},
		{
			ID:             "frida-server-log",
			Name:           "查看 server 日志",
			Category:       "Frida-server",
			Description:    "读取 /data/local/tmp/frida-server.log，用于定位 early end-of-stream。",
			RequiresDevice: true,
		},
		{
			ID:             "frida-logcat",
			Name:           "抓取 Frida logcat",
			Category:       "Frida-server",
			Description:    "从最近 logcat 中过滤 frida、crash、avc denied 等关键字。",
			RequiresDevice: true,
		},
		{
			ID:             "adb-forward-frida",
			Name:           "转发 Frida 端口",
			Category:       "Frida-server",
			Description:    "转发 tcp:27042 和 tcp:27043，排查 USB 通道问题。",
			RequiresDevice: true,
		},
		{
			ID:             "frida-devices",
			Name:           "Frida 设备列表",
			Category:       "Frida CLI",
			Description:    "执行 frida-ls-devices，确认 Frida CLI 能看到设备。",
			RequiresDevice: false,
		},
		{
			ID:             "frida-smoke-usb",
			Name:           "Frida 官方 smoke test",
			Category:       "Frida CLI",
			Description:    "按官方 Android 文档执行 frida-ps -U，验证 CLI 到 server 的链路。",
			RequiresDevice: false,
		},
		{
			ID:             "frida-processes",
			Name:           "Frida 进程列表",
			Category:       "Frida CLI",
			Description:    "执行 frida-ps -D <serial>，确认 Frida 与设备端 server 通信正常。",
			RequiresDevice: true,
		},
		{
			ID:             "frida-apps",
			Name:           "Frida 应用列表",
			Category:       "Frida CLI",
			Description:    "执行 frida-ps -D <serial> -a，列出 Frida 识别到的应用。",
			RequiresDevice: true,
		},
	}
}
