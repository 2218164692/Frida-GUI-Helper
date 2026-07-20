package diagnostics

import "strings"

type Finding struct {
	Code        string `json:"code"`
	Title       string `json:"title"`
	Reason      string `json:"reason"`
	Action      string `json:"action"`
	Recovery    string `json:"recovery"`
	Recoverable bool   `json:"recoverable"`
}

type rule struct {
	code        string
	containsAll []string
	containsAny []string
	finding     Finding
}

var rules = []rule{
	{
		code:        "spawn-timeout",
		containsAll: []string{"failed to spawn", "timed out"},
		finding: Finding{
			Title:       "Spawn 启动超时",
			Reason:      "目标进程没有在 Frida 等待时间内完成启动，常见于系统启动限制、应用卡死或启动阶段反调试。",
			Action:      "先手动启动目标 App，确认进入主界面后切换为 Attach 再重试；同时检查 logcat 中的崩溃或 ANR。",
			Recovery:    "attach",
			Recoverable: true,
		},
	},
	{
		code:        "gadget-required",
		containsAll: []string{"need gadget to attach on jailed android"},
		finding: Finding{
			Title:       "未连接设备端 frida-server",
			Reason:      "Frida CLI 将设备识别为无法直接注入的 Android 环境，通常是 server 未运行、连接到了错误端点或设备未提供 root 注入能力。",
			Action:      "检查 server 状态、版本和架构，使用 root 启动后再运行 Frida 官方 smoke test。",
			Recovery:    "server-check",
			Recoverable: true,
		},
	},
	{
		code:        "early-end-of-stream",
		containsAll: []string{"unexpected early end-of-stream"},
		finding: Finding{
			Title:       "Frida 通信提前断开",
			Reason:      "CLI 与设备端 server 或目标进程之间的通信被中断，可能由 server 崩溃、系统兼容性、反 Frida 或目标进程退出引起。",
			Action:      "刷新 server 状态并导出诊断包；检查 server 日志和 logcat。若仅 Spawn 失败，手动启动 App 后使用 Attach。",
			Recovery:    "server-check",
			Recoverable: true,
		},
	},
	{
		code:        "permission-denied",
		containsAny: []string{"permission denied", "operation not permitted"},
		finding: Finding{
			Title:       "权限不足",
			Reason:      "ADB shell、su 或目标文件没有获得执行当前操作所需的权限。",
			Action:      "确认设备已授权 ADB，在 root 管理器中允许 Shell 超级用户权限，并检查 frida-server 是否已 chmod 755。",
			Recovery:    "server-check",
			Recoverable: true,
		},
	},
	{
		code:        "version-mismatch",
		containsAny: []string{"version mismatch", "major versions must match", "server version does not match"},
		finding: Finding{
			Title:       "Frida 版本不匹配",
			Reason:      "电脑端 Frida CLI 与设备端 frida-server 版本不兼容。",
			Action:      "使用与当前 CLI 完全一致版本、并与设备 ABI 匹配的 frida-server，停止旧进程后重新推送启动。",
			Recovery:    "server-check",
			Recoverable: true,
		},
	},
	{
		code:        "attach-target-missing",
		containsAny: []string{"unable to find process with name", "process not found", "failed to attach: unable to find"},
		finding: Finding{
			Title:       "Attach 目标未运行",
			Reason:      "没有找到与当前 PID、进程名或包名匹配的运行进程。",
			Action:      "刷新进程列表，启动目标 App 后重新选择进程，或改用 Spawn。",
			Recovery:    "processes",
			Recoverable: true,
		},
	},
	{
		code:        "device-unavailable",
		containsAny: []string{"device not found", "device offline", "device unauthorized", "unable to connect to remote frida-server"},
		finding: Finding{
			Title:       "设备连接不可用",
			Reason:      "ADB 或 Frida 无法连接当前设备。",
			Action:      "重新连接 USB，确认设备授权状态，刷新设备列表并检查 frida-server。",
			Recovery:    "devices",
			Recoverable: true,
		},
	},
}

func Classify(source string, message string) (Finding, bool) {
	text := strings.ToLower(strings.TrimSpace(source + "\n" + message))
	if text == "" {
		return Finding{}, false
	}

	for _, candidate := range rules {
		if !matchesAll(text, candidate.containsAll) || !matchesAny(text, candidate.containsAny) {
			continue
		}
		finding := candidate.finding
		finding.Code = candidate.code
		return finding, true
	}
	return Finding{}, false
}

func matchesAll(text string, patterns []string) bool {
	for _, pattern := range patterns {
		if !strings.Contains(text, pattern) {
			return false
		}
	}
	return true
}

func matchesAny(text string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, pattern := range patterns {
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
}
