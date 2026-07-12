package scripts

type Template struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Source      string `json:"source"`
}

func List() []Template {
	return []Template{
		{
			ID:          "connection-probe",
			Name:        "连接探针",
			Category:    "诊断",
			Description: "最小脚本，只打印进程和运行时信息，用来确认基础注入链路。",
			Source: `console.log("[probe] script loaded");
console.log("[probe] pid=" + Process.id + " arch=" + Process.arch + " pointerSize=" + Process.pointerSize);
console.log("[probe] Java.available=" + Java.available);
setTimeout(function () {
    console.log("[probe] alive after 1000ms");
}, 1000);
`,
		},
		{
			ID:          "java-probe",
			Name:        "Java Runtime 探针",
			Category:    "诊断",
			Description: "只进入 Java.perform 并打印包名，用来判断 Java 层注入是否可用。",
			Source: `console.log("[java-probe] script loaded");
if (Java.available) {
    Java.perform(function () {
        try {
            const ActivityThread = Java.use("android.app.ActivityThread");
            const app = ActivityThread.currentApplication();
            const packageName = app ? app.getPackageName() : "<no currentApplication>";
            console.log("[java-probe] package=" + packageName);
        } catch (err) {
            console.log("[java-probe] failed: " + err);
        }
    });
} else {
    console.log("[java-probe] Java runtime is not available");
}
`,
		},
		{
			ID:          "memory-scan-patch",
			Name:        "Memory 扫描与补丁模板",
			Category:    "高阶 Native",
			Description: "使用 Memory.scan 搜索特征码，并可选 patch 命中地址附近的字节。",
			Source: `const moduleName = ""; // example: "libtarget.so"; leave empty to scan readable ranges
const pattern = "64 65 78 0a 30 ?? ?? 00"; // DEX magic example: dex\n0xx\0
const maxMatches = 16;
const patchEnabled = false;
const patchOffset = 0;
const patchBytes = [0x00, 0x00, 0x80, 0xd2]; // arm64: mov x0, #0

function rangesToScan() {
    if (moduleName.length > 0) {
        const mod = Process.getModuleByName(moduleName);
        return [{ base: mod.base, size: mod.size, protection: "module" }];
    }
    return Process.enumerateRanges({ protection: "r--", coalesce: true });
}

let matches = 0;
for (const range of rangesToScan()) {
    try {
        Memory.scan(range.base, range.size, pattern, {
            onMatch(address, size) {
                matches++;
                console.log("[scan] match #" + matches + " at " + address + " size=" + size);
                console.log(hexdump(address, { offset: 0, length: 96, header: true, ansi: false }));

                if (patchEnabled) {
                    const patchAddress = address.add(patchOffset);
                    Memory.patchCode(patchAddress, patchBytes.length, function (code) {
                        code.writeByteArray(patchBytes);
                    });
                    console.log("[scan] patched " + patchAddress);
                }

                if (matches >= maxMatches) {
                    return "stop";
                }
            },
            onError(reason) {
                console.log("[scan] error at " + range.base + ": " + reason);
            },
            onComplete() {}
        });
    } catch (err) {
        console.log("[scan] skipped range " + range.base + ": " + err);
    }
}
console.log("[scan] submitted scan jobs, matches are reported asynchronously");
`,
		},
		{
			ID:          "dex-memory-dump",
			Name:        "DEX 内存扫描与 Dump",
			Category:    "高阶 Native",
			Description: "扫描内存中的 DEX magic，按 header file_size 写出到 /data/local/tmp。",
			Source: `const outputDir = "/data/local/tmp/frida-dexdump";
const maxDexSize = 64 * 1024 * 1024;
const maxDumps = 8;
const pattern = "64 65 78 0a 30 ?? ?? 00";

function mkdir(path) {
    try {
        const mkdirPtr = Module.getGlobalExportByName("mkdir");
        const mkdir = new NativeFunction(mkdirPtr, "int", ["pointer", "int"]);
        mkdir(Memory.allocUtf8String(path), 0x1ff);
    } catch (err) {
        console.log("[dexdump] mkdir skipped: " + err);
    }
}

mkdir(outputDir);
let dumped = 0;
const seen = new Set();
const ranges = Process.enumerateRanges({ protection: "r--", coalesce: true });
for (const range of ranges) {
    Memory.scan(range.base, range.size, pattern, {
        onMatch(address) {
            const key = address.toString();
            if (seen.has(key) || dumped >= maxDumps) {
                return dumped >= maxDumps ? "stop" : undefined;
            }
            seen.add(key);

            try {
                const dexSize = address.add(0x20).readU32();
                if (dexSize <= 0x70 || dexSize > maxDexSize) {
                    console.log("[dexdump] suspicious size " + dexSize + " at " + address);
                    return;
                }

                const path = outputDir + "/classes-" + dumped + "-" + address + ".dex";
                const file = new File(path, "wb");
                file.write(address.readByteArray(dexSize));
                file.flush();
                file.close();
                dumped++;
                console.log("[dexdump] wrote " + dexSize + " bytes to " + path);
            } catch (err) {
                console.log("[dexdump] failed at " + address + ": " + err);
            }
        },
        onError(reason) {
            console.log("[dexdump] scan error: " + reason);
        },
        onComplete() {}
    });
}
console.log("[dexdump] scan submitted");
`,
		},
		{
			ID:          "stalker-native-trace",
			Name:        "Stalker Native 执行流追踪",
			Category:    "高阶 Trace",
			Description: "在目标 native 导出函数进入时追踪当前线程的 call/block 汇总。",
			Source: `const moduleName = "libc.so";
const exportName = "open";
const traceMs = 1500;

const mod = Process.getModuleByName(moduleName);
const target = mod.getExportByName(exportName);
console.log("[stalker] target=" + moduleName + "!" + exportName + " @ " + target);

Interceptor.attach(target, {
    onEnter(args) {
        this.threadId = Process.getCurrentThreadId();
        console.log("[stalker] follow thread " + this.threadId);
        Stalker.follow(this.threadId, {
            events: {
                call: true,
                ret: false,
                exec: false,
                block: true,
                compile: false
            },
            onCallSummary(summary) {
                console.log("[stalker] call summary=" + JSON.stringify(summary));
            }
        });

        const tid = this.threadId;
        setTimeout(function () {
            try {
                Stalker.unfollow(tid);
                Stalker.garbageCollect();
                console.log("[stalker] unfollow thread " + tid);
            } catch (err) {
                console.log("[stalker] unfollow failed: " + err);
            }
        }, traceMs);
    }
});
`,
		},
		{
			ID:          "java-register-class",
			Name:        "Java.registerClass 动态类",
			Category:    "高阶 Java",
			Description: "运行时创建 Java 类，可用于伪造接口、回调或 TrustManager。",
			Source: `if (Java.available) {
    Java.perform(function () {
        const Runnable = Java.use("java.lang.Runnable");
        const DynamicRunnable = Java.registerClass({
            name: "dev.frida.gui.DynamicRunnable",
            implements: [Runnable],
            fields: {
                tag: "java.lang.String"
            },
            methods: {
                $init: [{
                    returnType: "void",
                    argumentTypes: ["java.lang.String"],
                    implementation: function (tag) {
                        this.tag.value = tag;
                    }
                }],
                run: function () {
                    console.log("[registerClass] run tag=" + this.tag.value);
                }
            }
        });

        const instance = DynamicRunnable.$new("created-at-runtime");
        instance.run();
        console.log("[registerClass] created " + instance.$className);
    });
} else {
    console.log("[registerClass] Java runtime is not available");
}
`,
		},
		{
			ID:          "nativefunction-rpc",
			Name:        "NativeFunction 与 RPC",
			Category:    "高阶 Native",
			Description: "把 App 内 native 函数包装成本地可调用函数，并通过 rpc.exports 暴露。",
			Source: `const moduleName = "libc.so";
const exportName = "strlen";
const returnType = "ulong";
const argTypes = ["pointer"];

const mod = Process.getModuleByName(moduleName);
const target = mod.getExportByName(exportName);
const nativeCall = new NativeFunction(target, returnType, argTypes);
console.log("[rpc] target=" + moduleName + "!" + exportName + " @ " + target);

rpc.exports = {
    callstrlen(value) {
        const text = String(value);
        const ptrValue = Memory.allocUtf8String(text);
        return Number(nativeCall(ptrValue));
    }
};

setImmediate(function () {
    const sample = "frida-rpc";
    const result = nativeCall(Memory.allocUtf8String(sample));
    console.log("[rpc] local test strlen('" + sample + "')=" + result);
});
`,
		},
		{
			ID:          "interceptor-replace-writer",
			Name:        "Interceptor.replace 与 Writer",
			Category:    "高阶 Patch",
			Description: "用 Interceptor.replace 或 Arm64Writer 替换 native 函数入口。",
			Source: `const moduleName = "libtarget.so";
const exportName = "target_function";
const useArm64Writer = false;

const mod = Process.getModuleByName(moduleName);
const target = mod.getExportByName(exportName);
console.log("[replace] target=" + moduleName + "!" + exportName + " @ " + target);

if (useArm64Writer) {
    if (Process.arch !== "arm64") {
        throw new Error("Arm64Writer requires arm64 process, current=" + Process.arch);
    }

    const code = Memory.alloc(Process.pageSize);
    Memory.patchCode(code, 64, function (codePtr) {
        const writer = new Arm64Writer(codePtr, { pc: code });
        writer.putMovRegU64("x0", 0);
        writer.putRet();
        writer.flush();
    });
    Interceptor.replace(target, code);
    console.log("[replace] replaced with Arm64Writer thunk returning 0");
} else {
    const replacement = new NativeCallback(function () {
        console.log("[replace] replacement called");
        return 0;
    }, "int", []);
    Interceptor.replace(target, replacement);
    console.log("[replace] replaced with NativeCallback returning 0");
}
`,
		},
		{
			ID:          "ssl-pinning-basic",
			Name:        "SSL Pinning 基础绕过",
			Category:    "安全测试",
			Description: "覆盖常见 TrustManagerImpl、OkHttp CertificatePinner 和 SSLContext.init 校验路径。",
			Source: `if (Java.available) {
    Java.perform(function () {
        console.log("[ssl] installing hooks");

        try {
            const TrustManagerImpl = Java.use("com.android.org.conscrypt.TrustManagerImpl");
            TrustManagerImpl.verifyChain.implementation = function (untrustedChain, trustAnchorChain, host, clientAuth, ocspData, tlsSctData) {
                console.log("[ssl] TrustManagerImpl.verifyChain host=" + host);
                return untrustedChain;
            };
        } catch (err) {
            console.log("[ssl] TrustManagerImpl hook skipped: " + err);
        }

        try {
            const CertificatePinner = Java.use("okhttp3.CertificatePinner");
            CertificatePinner.check.overload("java.lang.String", "java.util.List").implementation = function (host, peerCertificates) {
                console.log("[ssl] OkHttp CertificatePinner.check host=" + host);
                return;
            };
        } catch (err) {
            console.log("[ssl] OkHttp hook skipped: " + err);
        }

        try {
            const X509TrustManager = Java.use("javax.net.ssl.X509TrustManager");
            const SSLContext = Java.use("javax.net.ssl.SSLContext");
            const TrustManager = Java.registerClass({
                name: "dev.frida.gui.helper.TrustManager",
                implements: [X509TrustManager],
                methods: {
                    checkClientTrusted: function (chain, authType) {},
                    checkServerTrusted: function (chain, authType) {},
                    getAcceptedIssuers: function () { return []; }
                }
            });
            SSLContext.init.overload(
                "[Ljavax.net.ssl.KeyManager;",
                "[Ljavax.net.ssl.TrustManager;",
                "java.security.SecureRandom"
            ).implementation = function (keyManagers, trustManagers, secureRandom) {
                console.log("[ssl] SSLContext.init replaced TrustManager");
                return this.init(keyManagers, [TrustManager.$new()], secureRandom);
            };
        } catch (err) {
            console.log("[ssl] SSLContext hook skipped: " + err);
        }
    });
} else {
    console.log("[ssl] Java runtime is not available");
}
`,
		},
		{
			ID:          "intent-monitor",
			Name:        "Intent 传递监控",
			Category:    "信息探测",
			Description: "记录 Activity.startActivity 与 ContextWrapper.startActivity 的 Intent 内容。",
			Source: `if (Java.available) {
    Java.perform(function () {
        function dumpIntent(prefix, intent) {
            try {
                console.log(prefix + " action=" + intent.getAction());
                console.log(prefix + " data=" + intent.getDataString());
                console.log(prefix + " component=" + intent.getComponent());
                console.log(prefix + " extras=" + intent.getExtras());
            } catch (err) {
                console.log(prefix + " dump failed: " + err);
            }
        }

        try {
            const Activity = Java.use("android.app.Activity");
            Activity.startActivity.overload("android.content.Intent").implementation = function (intent) {
                dumpIntent("[intent] Activity.startActivity", intent);
                return this.startActivity(intent);
            };
        } catch (err) {
            console.log("[intent] Activity hook skipped: " + err);
        }

        try {
            const ContextWrapper = Java.use("android.content.ContextWrapper");
            ContextWrapper.startActivity.overload("android.content.Intent").implementation = function (intent) {
                dumpIntent("[intent] ContextWrapper.startActivity", intent);
                return this.startActivity(intent);
            };
        } catch (err) {
            console.log("[intent] ContextWrapper hook skipped: " + err);
        }
    });
} else {
    console.log("[intent] Java runtime is not available");
}
`,
		},
		{
			ID:          "loaded-classes",
			Name:        "已加载类枚举",
			Category:    "信息探测",
			Description: "按关键字过滤并打印当前 Java 运行时已加载类。",
			Source: `if (Java.available) {
    Java.perform(function () {
        const keyword = "";
        Java.enumerateLoadedClasses({
            onMatch: function (className) {
                if (!keyword || className.indexOf(keyword) !== -1) {
                    console.log("[class] " + className);
                }
            },
            onComplete: function () {
                console.log("[class] enumeration complete");
            }
        });
    });
} else {
    console.log("[class] Java runtime is not available");
}
`,
		},
	}
}
