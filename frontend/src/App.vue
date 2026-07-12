<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import {
  AlertTriangle,
  CheckCircle2,
  Cpu,
  Download,
  FileCode2,
  ListTree,
  MonitorSmartphone,
  Play,
  PlugZap,
  RefreshCw,
  Search,
  Send,
  Server,
  ShieldCheck,
  Square,
  Terminal,
  Trash2,
  Upload,
  Wrench,
  Usb
} from "@lucide/vue";
import {
  api,
  subscribeLogs,
  type AndroidApp,
  type AndroidProcess,
  type Device,
  type LogEntry,
  type OperationTemplate,
  type ScriptTemplate,
  type SessionInfo,
  type SystemStatus,
  type ToolStatus
} from "./lib/api";

type ViewKey = "devices" | "processes" | "scripts" | "logs";
type RunMode = "attach" | "spawn";
type TargetKind = "pid" | "name" | "package";
type LevelFilter = "all" | "info" | "warn" | "error";

const views: Array<{ key: ViewKey; label: string; icon: unknown }> = [
  { key: "devices", label: "设备", icon: MonitorSmartphone },
  { key: "processes", label: "进程", icon: ListTree },
  { key: "scripts", label: "脚本", icon: FileCode2 },
  { key: "logs", label: "日志", icon: Terminal }
];

const activeView = ref<ViewKey>("devices");
const busy = ref("");
const notice = ref("");

const systemStatus = ref<SystemStatus | null>(null);
const devices = ref<Device[]>([]);
const selectedSerial = ref("");
const includeSystemApps = ref(false);
const apps = ref<AndroidApp[]>([]);
const processes = ref<AndroidProcess[]>([]);
const processQuery = ref("");
const appQuery = ref("");
const processTab = ref<"processes" | "apps">("processes");

const scripts = ref<ScriptTemplate[]>([]);
const operations = ref<OperationTemplate[]>([]);
const selectedScriptID = ref("");
const editorSource = ref("");
const runMode = ref<RunMode>("attach");
const targetKind = ref<TargetKind>("name");
const target = ref("");

const sessions = ref<SessionInfo[]>([]);
const logs = ref<LogEntry[]>([]);
const logQuery = ref("");
const logLevel = ref<LevelFilter>("all");

const fridaLocalPath = ref("");
const fridaRemotePath = ref("/data/local/tmp/frida-server");
const forceRestart = ref(false);

let unsubscribeLogs: () => void = () => {};

const currentTitle = computed(() => views.find((view) => view.key === activeView.value)?.label ?? "");
const selectedDevice = computed(() => devices.value.find((device) => device.serial === selectedSerial.value));
const selectedDeviceLabel = computed(() => {
  if (!selectedDevice.value) {
    return "未选择设备";
  }
  const model = selectedDevice.value.model || selectedDevice.value.product || selectedDevice.value.state;
  return `${selectedDevice.value.serial}${model ? ` · ${model}` : ""}`;
});

const activeScript = computed(() => scripts.value.find((script) => script.id === selectedScriptID.value));
const activeScriptName = computed(() => activeScript.value?.name || "自定义脚本");
const canRunScript = computed(() => Boolean(selectedSerial.value && target.value.trim() && editorSource.value.trim()));

const filteredProcesses = computed(() => {
  const query = processQuery.value.trim().toLowerCase();
  if (!query) {
    return processes.value;
  }
  return processes.value.filter((process) =>
    [process.name, process.package, process.user, String(process.pid)].some((value) =>
      value.toLowerCase().includes(query)
    )
  );
});

const filteredApps = computed(() => {
  const query = appQuery.value.trim().toLowerCase();
  if (!query) {
    return apps.value;
  }
  return apps.value.filter((app) =>
    [app.name, app.package, app.path].some((value) => value.toLowerCase().includes(query))
  );
});

const filteredLogs = computed(() => {
  const query = logQuery.value.trim().toLowerCase();
  return logs.value.filter((entry) => {
    const levelMatches = logLevel.value === "all" || entry.level === logLevel.value;
    const textMatches =
      !query ||
      [entry.time, entry.level, entry.source, entry.message].some((value) =>
        String(value).toLowerCase().includes(query)
      );
    return levelMatches && textMatches;
  });
});

const runningSessions = computed(() => sessions.value.filter((session) => session.running));

onMounted(async () => {
  unsubscribeLogs = subscribeLogs((entry) => {
    logs.value = [...logs.value, entry].slice(-1000);
  });
  await bootstrap();
});

onUnmounted(() => {
  unsubscribeLogs();
});

async function bootstrap() {
  await Promise.all([refreshStatus(), refreshScripts(), refreshOperations(), refreshLogs()]);
  await refreshDevices();
  await refreshSessions();
}

async function withBusy<T>(key: string, action: () => Promise<T>): Promise<T | undefined> {
  busy.value = key;
  notice.value = "";
  try {
    return await action();
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    notice.value = message;
    appendLocalLog("error", "ui", message);
    return undefined;
  } finally {
    busy.value = "";
  }
}

async function refreshStatus() {
  await withBusy("status", async () => {
    systemStatus.value = await api.getSystemStatus();
  });
}

async function refreshDevices() {
  await withBusy("devices", async () => {
    devices.value = await api.listDevices();
    if (!selectedSerial.value && devices.value.length > 0) {
      selectedSerial.value = devices.value.find((device) => device.isAuthorized)?.serial ?? devices.value[0].serial;
    }
  });
}

async function refreshProcesses() {
  await withBusy("processes", async () => {
    processes.value = await api.listProcesses(selectedSerial.value);
  });
}

async function refreshApps() {
  await withBusy("apps", async () => {
    apps.value = await api.listApps(selectedSerial.value, includeSystemApps.value);
  });
}

async function refreshScripts() {
  await withBusy("scripts", async () => {
    scripts.value = await api.listScripts();
    if (!selectedScriptID.value && scripts.value.length > 0) {
      selectScript(scripts.value[0]);
    }
  });
}

async function importScriptFile() {
  await withBusy("import-script", async () => {
    const imported = await api.importScriptFile();
    if (!imported?.source) {
      return;
    }
    const script: ScriptTemplate = {
      id: `imported:${Date.now()}:${imported.path}`,
      name: imported.name || "imported-script.js",
      category: "自定义",
      description: imported.path || "本地导入脚本",
      source: imported.source
    };
    scripts.value = [script, ...scripts.value.filter((item) => item.id !== script.id)];
    selectScript(script);
    appendLocalLog("info", "ui", `已导入脚本: ${script.name}`);
  });
}

async function refreshOperations() {
  await withBusy("operations", async () => {
    operations.value = await api.listOperations();
  });
}

async function refreshLogs() {
  await withBusy("logs", async () => {
    logs.value = await api.getLogs();
  });
}

async function refreshSessions() {
  sessions.value = await api.listSessions();
}

function selectScript(script: ScriptTemplate) {
  selectedScriptID.value = script.id;
  editorSource.value = script.source;
  activeView.value = "scripts";
}

async function startFridaServer() {
  await withBusy("frida-server", async () => {
    await api.startFridaServer({
      deviceSerial: selectedSerial.value,
      localPath: fridaLocalPath.value.trim(),
      remotePath: fridaRemotePath.value.trim(),
      forceRestart: forceRestart.value
    });
    await refreshStatus();
  });
}

function prepareAttach(process: AndroidProcess) {
  runMode.value = "attach";
  targetKind.value = "pid";
  target.value = String(process.pid);
  activeView.value = "scripts";
}

async function attachProcess(process: AndroidProcess) {
  prepareAttach(process);
  await runCurrentScript();
}

function prepareSpawn(app: AndroidApp) {
  runMode.value = "spawn";
  targetKind.value = "package";
  target.value = app.package;
  activeView.value = "scripts";
}

async function spawnApp(app: AndroidApp) {
  prepareSpawn(app);
  await runCurrentScript();
}

async function runCurrentScript() {
  await withBusy("run-script", async () => {
    const session = await api.runScript({
      deviceSerial: selectedSerial.value,
      mode: runMode.value,
      targetKind: runMode.value === "spawn" ? "package" : targetKind.value,
      target: target.value.trim(),
      scriptName: activeScriptName.value,
      scriptSource: editorSource.value
    });
    appendLocalLog("info", "ui", `已创建 Frida 会话 ${session.id}`);
    await refreshSessions();
  });
}

async function runOperation(operation: OperationTemplate) {
  await withBusy(`operation-${operation.id}`, async () => {
    await api.runOperation({
      id: operation.id,
      deviceSerial: selectedSerial.value
    });
    activeView.value = "logs";
  });
}

async function stopSession(sessionID: string) {
  await withBusy(`stop-${sessionID}`, async () => {
    await api.stopSession(sessionID);
    await refreshSessions();
  });
}

async function clearLogs() {
  await withBusy("clear-logs", async () => {
    await api.clearLogs();
    logs.value = await api.getLogs();
  });
}

function exportLogs() {
  const content = filteredLogs.value
    .map((entry) => `[${entry.time}] ${entry.level.toUpperCase()} ${entry.source}: ${entry.message}`)
    .join("\n");
  const blob = new Blob([content], { type: "text/plain;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `frida-gui-helper-${new Date().toISOString().replace(/[:.]/g, "-")}.log`;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

function toolClass(tool?: ToolStatus) {
  return tool?.found ? "ok" : "bad";
}

function toolLabel(tool?: ToolStatus) {
  if (!tool) {
    return "未检测";
  }
  const detail = tool.version || tool.error || "未检测";
  return tool.source ? `${detail} · ${tool.source}` : detail;
}

function appendLocalLog(level: LogEntry["level"], source: string, message: string) {
  logs.value = [
    ...logs.value,
    {
      time: new Date().toLocaleTimeString("zh-CN", { hour12: false }),
      level,
      source,
      message
    }
  ].slice(-1000);
}
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar">
      <div class="brand">
        <div class="brand-mark"><Cpu :size="20" /></div>
        <div class="brand-text">
          <strong>Frida GUI</strong>
          <span>Helper</span>
        </div>
      </div>

      <nav class="nav-list" aria-label="主导航">
        <button
          v-for="view in views"
          :key="view.key"
          class="nav-button"
          :class="{ active: activeView === view.key }"
          type="button"
          @click="activeView = view.key"
        >
          <component :is="view.icon" :size="18" />
          <span class="nav-label">{{ view.label }}</span>
        </button>
      </nav>

      <div class="sidebar-footer">
        <div class="mini-status" :class="toolClass(systemStatus?.adb)">
          <Usb :size="15" />
          <span>ADB</span>
        </div>
        <div class="mini-status" :class="toolClass(systemStatus?.frida)">
          <ShieldCheck :size="15" />
          <span>Frida</span>
        </div>
      </div>
    </aside>

    <main class="main">
      <header class="topbar">
        <div class="title-block">
          <h1>{{ currentTitle }}</h1>
          <span>{{ selectedDeviceLabel }}</span>
        </div>
        <div class="top-actions">
          <button class="icon-button" type="button" title="刷新状态" @click="bootstrap">
            <RefreshCw :size="18" :class="{ spin: busy !== '' }" />
          </button>
          <button class="text-button" type="button" @click="activeView = 'scripts'">
            <Send :size="17" />
            <span>运行脚本</span>
          </button>
        </div>
      </header>

      <div v-if="notice" class="notice">
        <AlertTriangle :size="18" />
        <span>{{ notice }}</span>
      </div>

      <section v-if="activeView === 'devices'" class="workspace grid-2">
        <div class="panel">
          <div class="panel-header">
            <div>
              <h2>环境状态</h2>
              <p>ADB、Frida CLI 和 Python 可用性</p>
            </div>
            <button class="icon-button" type="button" title="刷新环境" @click="refreshStatus">
              <RefreshCw :size="17" />
            </button>
          </div>

          <div class="tool-grid">
            <div class="tool-row" :class="toolClass(systemStatus?.adb)">
              <Usb :size="18" />
              <strong>ADB</strong>
              <span :title="systemStatus?.adb.path">{{ toolLabel(systemStatus?.adb) }}</span>
            </div>
            <div class="tool-row" :class="toolClass(systemStatus?.frida)">
              <ShieldCheck :size="18" />
              <strong>Frida</strong>
              <span :title="systemStatus?.frida.path">{{ toolLabel(systemStatus?.frida) }}</span>
            </div>
            <div class="tool-row" :class="toolClass(systemStatus?.python)">
              <Terminal :size="18" />
              <strong>Python</strong>
              <span :title="systemStatus?.python.path">{{ toolLabel(systemStatus?.python) }}</span>
            </div>
          </div>
        </div>

        <div class="panel">
          <div class="panel-header">
            <div>
              <h2>Frida-server</h2>
              <p>推送、授权并在设备端启动</p>
            </div>
            <button class="icon-button" type="button" title="启动 Frida-server" @click="startFridaServer">
              <Play :size="17" />
            </button>
          </div>

          <div class="form-stack">
            <label>
              <span>本地二进制</span>
              <input v-model="fridaLocalPath" type="text" placeholder="留空使用 tools/frida-server 内置文件" />
            </label>
            <label>
              <span>设备路径</span>
              <input v-model="fridaRemotePath" type="text" />
            </label>
            <label class="check-row">
              <input v-model="forceRestart" type="checkbox" />
              <span>强制重启已有 frida-server</span>
            </label>
            <button class="primary-button" type="button" :disabled="!selectedSerial" @click="startFridaServer">
              <Server :size="17" />
              <span>启动守护</span>
            </button>
          </div>
        </div>

        <div class="panel span-2">
          <div class="panel-header">
            <div>
              <h2>Android 设备</h2>
              <p>通过 USB/ADB 扫描</p>
            </div>
            <button class="icon-button" type="button" title="刷新设备" @click="refreshDevices">
              <RefreshCw :size="17" />
            </button>
          </div>

          <div class="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>序列号</th>
                  <th>状态</th>
                  <th>型号</th>
                  <th>产品</th>
                  <th class="cell-action">操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="devices.length === 0">
                  <td colspan="5" class="empty">暂无设备</td>
                </tr>
                <tr v-for="device in devices" :key="device.serial" :class="{ selected: selectedSerial === device.serial }">
                  <td>{{ device.serial }}</td>
                  <td>
                    <span class="state-pill" :class="{ ok: device.isAuthorized }">
                      {{ device.state }}
                    </span>
                  </td>
                  <td>{{ device.model || "-" }}</td>
                  <td>{{ device.product || "-" }}</td>
                  <td class="cell-action">
                    <button class="small-button" type="button" @click="selectedSerial = device.serial">选择</button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </section>

      <section v-else-if="activeView === 'processes'" class="workspace">
        <div class="toolbar-band">
          <div class="segmented">
            <button type="button" :class="{ active: processTab === 'processes' }" @click="processTab = 'processes'">
              运行进程
            </button>
            <button type="button" :class="{ active: processTab === 'apps' }" @click="processTab = 'apps'">应用包</button>
          </div>
          <div class="toolbar-right">
            <label class="search-box">
              <Search :size="17" />
              <input
                v-if="processTab === 'processes'"
                v-model="processQuery"
                type="search"
                placeholder="过滤进程、PID、包名"
              />
              <input v-else v-model="appQuery" type="search" placeholder="过滤应用包名" />
            </label>
            <button
              class="icon-button"
              type="button"
              title="刷新列表"
              @click="processTab === 'processes' ? refreshProcesses() : refreshApps()"
            >
              <RefreshCw :size="17" />
            </button>
          </div>
        </div>

        <div v-if="processTab === 'processes'" class="panel">
          <div class="panel-header">
            <div>
              <h2>进程列表</h2>
              <p>{{ filteredProcesses.length }} / {{ processes.length }} 个进程</p>
            </div>
            <button class="primary-button" type="button" :disabled="!selectedSerial" @click="refreshProcesses">
              <RefreshCw :size="17" />
              <span>刷新进程</span>
            </button>
          </div>
          <div class="table-wrap tall">
            <table>
              <thead>
                <tr>
                  <th>PID</th>
                  <th>用户</th>
                  <th>进程名</th>
                  <th>包名</th>
                  <th class="cell-action">操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="filteredProcesses.length === 0">
                  <td colspan="5" class="empty">暂无进程</td>
                </tr>
                <tr v-for="process in filteredProcesses" :key="`${process.pid}-${process.name}`">
                  <td>{{ process.pid }}</td>
                  <td>{{ process.user || "-" }}</td>
                  <td :title="process.name">{{ process.name }}</td>
                  <td :title="process.package">{{ process.package || "-" }}</td>
                  <td class="cell-action action-group">
                    <button class="small-button" type="button" title="填入目标" @click="prepareAttach(process)">
                      <PlugZap :size="14" />
                    </button>
                    <button class="small-button strong" type="button" @click="attachProcess(process)">Attach</button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <div v-else class="panel">
          <div class="panel-header">
            <div>
              <h2>应用包列表</h2>
              <p>{{ filteredApps.length }} / {{ apps.length }} 个应用</p>
            </div>
            <div class="header-controls">
              <label class="check-row compact">
                <input v-model="includeSystemApps" type="checkbox" @change="refreshApps" />
                <span>包含系统应用</span>
              </label>
              <button class="primary-button" type="button" :disabled="!selectedSerial" @click="refreshApps">
                <RefreshCw :size="17" />
                <span>刷新应用</span>
              </button>
            </div>
          </div>
          <div class="table-wrap tall">
            <table>
              <thead>
                <tr>
                  <th>包名</th>
                  <th>路径</th>
                  <th>类型</th>
                  <th class="cell-action">操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="filteredApps.length === 0">
                  <td colspan="4" class="empty">暂无应用</td>
                </tr>
                <tr v-for="app in filteredApps" :key="app.package">
                  <td :title="app.package">{{ app.package }}</td>
                  <td :title="app.path">{{ app.path || "-" }}</td>
                  <td>{{ app.system ? "系统" : "用户" }}</td>
                  <td class="cell-action action-group">
                    <button class="small-button" type="button" title="填入目标" @click="prepareSpawn(app)">
                      <PlugZap :size="14" />
                    </button>
                    <button class="small-button strong" type="button" @click="spawnApp(app)">Spawn</button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </section>

      <section v-else-if="activeView === 'scripts'" class="workspace script-layout">
        <div class="panel script-list">
          <div class="panel-header">
            <div>
              <h2>Script Hub</h2>
              <p>{{ scripts.length }} 个内置模板</p>
            </div>
            <div class="header-controls">
              <button class="icon-button" type="button" title="导入脚本" @click="importScriptFile">
                <Upload :size="17" />
              </button>
              <button class="icon-button" type="button" title="刷新脚本" @click="refreshScripts">
                <RefreshCw :size="17" />
              </button>
            </div>
          </div>

          <button
            v-for="script in scripts"
            :key="script.id"
            class="script-item"
            :class="{ active: selectedScriptID === script.id }"
            type="button"
            @click="selectScript(script)"
          >
            <span>{{ script.category }}</span>
            <strong>{{ script.name }}</strong>
            <small>{{ script.description }}</small>
          </button>

          <div class="hub-divider">
            <Wrench :size="15" />
            <span>常用操作</span>
          </div>

          <button
            v-for="operation in operations"
            :key="operation.id"
            class="operation-item"
            type="button"
            :disabled="operation.requiresDevice && !selectedSerial"
            @click="runOperation(operation)"
          >
            <span>{{ operation.category }}</span>
            <strong>{{ operation.name }}</strong>
            <small>{{ operation.description }}</small>
          </button>
        </div>

        <div class="panel editor-panel">
          <div class="panel-header">
            <div>
              <h2>脚本编辑器</h2>
              <p>{{ activeScriptName }}</p>
            </div>
            <button class="primary-button" type="button" :disabled="!canRunScript" @click="runCurrentScript">
              <Play :size="17" />
              <span>注入</span>
            </button>
          </div>

          <div class="run-grid">
            <label>
              <span>运行模式</span>
              <div class="segmented wide">
                <button type="button" :class="{ active: runMode === 'attach' }" @click="runMode = 'attach'">Attach</button>
                <button type="button" :class="{ active: runMode === 'spawn' }" @click="runMode = 'spawn'">Spawn</button>
              </div>
            </label>
            <label>
              <span>目标类型</span>
              <select v-model="targetKind" :disabled="runMode === 'spawn'">
                <option value="name">进程名</option>
                <option value="pid">PID</option>
                <option value="package">包名</option>
              </select>
            </label>
            <label class="target-input">
              <span>目标</span>
              <input v-model="target" type="text" placeholder="com.example.app 或 1234" />
            </label>
          </div>

          <textarea v-model="editorSource" spellcheck="false" class="code-editor" />
        </div>

        <div class="panel sessions-panel">
          <div class="panel-header">
            <div>
              <h2>会话</h2>
              <p>{{ runningSessions.length }} 个运行中</p>
            </div>
            <button class="icon-button" type="button" title="刷新会话" @click="refreshSessions">
              <RefreshCw :size="17" />
            </button>
          </div>

          <div class="session-list">
            <div v-if="sessions.length === 0" class="empty compact-empty">暂无会话</div>
            <div v-for="session in sessions" :key="session.id" class="session-row">
              <div>
                <strong>{{ session.scriptName }}</strong>
                <span>{{ session.mode }} · {{ session.target }}</span>
              </div>
              <button
                class="icon-button danger"
                type="button"
                title="停止会话"
                :disabled="!session.running"
                @click="stopSession(session.id)"
              >
                <Square :size="16" />
              </button>
            </div>
          </div>
        </div>
      </section>

      <section v-else class="workspace">
        <div class="toolbar-band">
          <div class="segmented">
            <button type="button" :class="{ active: logLevel === 'all' }" @click="logLevel = 'all'">全部</button>
            <button type="button" :class="{ active: logLevel === 'info' }" @click="logLevel = 'info'">Info</button>
            <button type="button" :class="{ active: logLevel === 'warn' }" @click="logLevel = 'warn'">Warn</button>
            <button type="button" :class="{ active: logLevel === 'error' }" @click="logLevel = 'error'">Error</button>
          </div>
          <div class="toolbar-right">
            <label class="search-box">
              <Search :size="17" />
              <input v-model="logQuery" type="search" placeholder="过滤日志" />
            </label>
            <button class="icon-button" type="button" title="导出日志" @click="exportLogs">
              <Download :size="17" />
            </button>
            <button class="icon-button danger" type="button" title="清空日志" @click="clearLogs">
              <Trash2 :size="17" />
            </button>
          </div>
        </div>

        <div class="log-console">
          <div v-if="filteredLogs.length === 0" class="empty">暂无日志</div>
          <div v-for="(entry, index) in filteredLogs" :key="`${entry.time}-${index}`" class="log-line" :class="entry.level">
            <span class="log-time">{{ entry.time }}</span>
            <span class="log-level">{{ entry.level }}</span>
            <span class="log-source">{{ entry.source }}</span>
            <span class="log-message">{{ entry.message }}</span>
          </div>
        </div>
      </section>
    </main>
  </div>
</template>

<style scoped>
.app-shell {
  display: grid;
  grid-template-columns: 244px minmax(0, 1fr);
  width: 100%;
  height: 100vh;
  min-width: 0;
}

.sidebar {
  display: flex;
  flex-direction: column;
  min-width: 0;
  padding: 18px 14px;
  color: #eef3f1;
  background: #252b2e;
  border-right: 1px solid #1a1f22;
}

.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 44px;
  margin-bottom: 20px;
}

.brand-mark {
  display: grid;
  width: 34px;
  height: 34px;
  place-items: center;
  color: #ffffff;
  background: #167c64;
  border-radius: 8px;
}

.brand-text {
  display: grid;
  min-width: 0;
  line-height: 1.15;
}

.brand-text strong,
.brand-text span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.brand-text span {
  color: #9fb3ad;
  font-size: 12px;
}

.nav-list {
  display: grid;
  gap: 6px;
}

.nav-button {
  display: grid;
  grid-template-columns: 24px minmax(0, 1fr);
  align-items: center;
  width: 100%;
  height: 42px;
  padding: 0 10px;
  color: #cdd7d4;
  text-align: left;
  background: transparent;
  border: 0;
  border-radius: 8px;
}

.nav-button:hover,
.nav-button.active {
  color: #ffffff;
  background: #354044;
}

.nav-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sidebar-footer {
  display: grid;
  gap: 8px;
  margin-top: auto;
}

.mini-status {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 32px;
  padding: 0 10px;
  color: #f3c96d;
  background: #30363a;
  border-radius: 8px;
}

.mini-status.ok {
  color: #85d3bd;
}

.mini-status.bad {
  color: #ffb4a9;
}

.main {
  display: flex;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
}

.topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  min-height: 76px;
  padding: 14px 22px;
  background: #ffffff;
  border-bottom: 1px solid #dde3ea;
}

.title-block {
  display: grid;
  min-width: 0;
  gap: 4px;
}

.title-block h1 {
  margin: 0;
  overflow: hidden;
  color: #20252d;
  font-size: 22px;
  font-weight: 700;
  line-height: 1.2;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.title-block span {
  overflow: hidden;
  color: #62707d;
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.top-actions,
.toolbar-right,
.header-controls,
.action-group {
  display: flex;
  align-items: center;
  gap: 8px;
}

.notice {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 42px;
  padding: 0 22px;
  color: #8a321f;
  background: #fff3ed;
  border-bottom: 1px solid #ffd6c8;
}

.notice span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.workspace {
  display: grid;
  min-height: 0;
  gap: 16px;
  padding: 18px 22px;
  overflow: auto;
  align-content: start;
}

.grid-2 {
  grid-template-columns: minmax(0, 1fr) minmax(320px, 420px);
}

.span-2 {
  grid-column: 1 / -1;
}

.panel {
  min-width: 0;
  overflow: hidden;
  background: #ffffff;
  border: 1px solid #dde3ea;
  border-radius: 8px;
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 62px;
  padding: 14px 16px;
  border-bottom: 1px solid #e7ebf0;
}

.panel-header h2 {
  margin: 0;
  color: #20252d;
  font-size: 16px;
  line-height: 1.25;
}

.panel-header p {
  max-width: 720px;
  margin: 4px 0 0;
  overflow: hidden;
  color: #6d7985;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.icon-button,
.small-button,
.primary-button,
.text-button,
.segmented button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  height: 36px;
  color: #27313b;
  background: #ffffff;
  border: 1px solid #cad2dc;
  border-radius: 7px;
}

.icon-button {
  width: 36px;
  padding: 0;
}

.text-button,
.primary-button {
  padding: 0 13px;
}

.primary-button,
.small-button.strong {
  color: #ffffff;
  background: #167c64;
  border-color: #167c64;
}

.primary-button:hover,
.small-button.strong:hover {
  background: #126b56;
}

.danger {
  color: #b42318;
}

.small-button {
  min-width: 32px;
  height: 30px;
  padding: 0 9px;
  font-size: 12px;
}

.tool-grid {
  display: grid;
  gap: 10px;
  padding: 16px;
}

.tool-row {
  display: grid;
  grid-template-columns: 24px 74px minmax(0, 1fr);
  align-items: center;
  gap: 8px;
  min-height: 42px;
  padding: 0 12px;
  color: #8a321f;
  background: #fff7f3;
  border: 1px solid #ffd8ca;
  border-radius: 8px;
}

.tool-row.ok {
  color: #0f684f;
  background: #f0faf6;
  border-color: #bfe8d8;
}

.tool-row span {
  min-width: 0;
  overflow: hidden;
  color: #44505b;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.form-stack {
  display: grid;
  gap: 12px;
  padding: 16px;
}

label {
  display: grid;
  gap: 7px;
  min-width: 0;
  color: #53606b;
  font-size: 12px;
}

input,
select,
textarea {
  width: 100%;
  min-width: 0;
  color: #20252d;
  background: #ffffff;
  border: 1px solid #cfd6df;
  border-radius: 7px;
  outline: none;
}

input,
select {
  height: 36px;
  padding: 0 10px;
}

textarea {
  resize: none;
}

input:focus,
select:focus,
textarea:focus {
  border-color: #167c64;
  box-shadow: 0 0 0 3px rgba(22, 124, 100, 0.12);
}

.check-row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 28px;
}

.check-row input {
  width: 16px;
  height: 16px;
}

.check-row.compact {
  font-size: 12px;
}

.toolbar-band {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-width: 0;
  padding: 12px;
  background: #ffffff;
  border: 1px solid #dde3ea;
  border-radius: 8px;
}

.segmented {
  display: inline-flex;
  min-width: 0;
  padding: 3px;
  background: #eef2f5;
  border-radius: 8px;
}

.segmented.wide {
  width: 100%;
}

.segmented button {
  min-width: 74px;
  height: 30px;
  padding: 0 12px;
  color: #566371;
  background: transparent;
  border: 0;
}

.segmented.wide button {
  width: 50%;
}

.segmented button.active {
  color: #16221f;
  background: #ffffff;
  box-shadow: 0 1px 3px rgba(36, 44, 52, 0.12);
}

.search-box {
  position: relative;
  display: block;
  width: min(360px, 40vw);
}

.search-box svg {
  position: absolute;
  top: 9px;
  left: 10px;
  color: #6a7783;
}

.search-box input {
  padding-left: 34px;
}

.table-wrap {
  width: 100%;
  overflow: auto;
}

.table-wrap.tall {
  max-height: calc(100vh - 238px);
}

table {
  width: 100%;
  min-width: 720px;
  border-collapse: collapse;
  table-layout: fixed;
}

th,
td {
  height: 42px;
  padding: 0 12px;
  overflow: hidden;
  text-align: left;
  text-overflow: ellipsis;
  white-space: nowrap;
  border-bottom: 1px solid #ebeff3;
}

th {
  color: #64717e;
  font-size: 12px;
  font-weight: 700;
  background: #f8fafc;
}

tr.selected td {
  background: #effaf6;
}

.cell-action {
  width: 150px;
  text-align: right;
}

.cell-action.action-group {
  justify-content: flex-end;
}

.state-pill {
  display: inline-flex;
  align-items: center;
  height: 24px;
  padding: 0 8px;
  color: #8a321f;
  background: #fff3ed;
  border-radius: 999px;
}

.state-pill.ok {
  color: #0f684f;
  background: #e9f8f2;
}

.empty {
  display: grid;
  min-height: 120px;
  place-items: center;
  color: #7a8793;
}

td.empty {
  text-align: center;
}

.compact-empty {
  min-height: 60px;
}

.script-layout {
  grid-template-columns: minmax(220px, 300px) minmax(0, 1fr);
  grid-template-rows: minmax(0, 1fr) auto;
  height: 100%;
  align-content: stretch;
}

.script-list {
  display: flex;
  min-height: 0;
  flex-direction: column;
  grid-row: 1 / 3;
  overflow-y: auto;
}

.script-item,
.operation-item {
  display: grid;
  gap: 5px;
  width: calc(100% - 20px);
  margin: 10px 10px 0;
  padding: 12px;
  text-align: left;
  background: #ffffff;
  border: 1px solid #dfe5eb;
  border-radius: 8px;
}

.script-item:hover,
.script-item.active,
.operation-item:hover {
  border-color: #167c64;
  box-shadow: 0 0 0 3px rgba(22, 124, 100, 0.1);
}

.script-item span,
.script-item small,
.script-item strong,
.operation-item span,
.operation-item small,
.operation-item strong {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}

.script-item span,
.operation-item span {
  color: #a45f16;
  font-size: 11px;
}

.script-item strong,
.operation-item strong {
  color: #20252d;
  font-size: 14px;
  white-space: nowrap;
}

.script-item small,
.operation-item small {
  display: -webkit-box;
  color: #62707d;
  font-size: 12px;
  line-height: 1.45;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.operation-item {
  background: #f8fafc;
}

.operation-item:disabled {
  cursor: not-allowed;
}

.hub-divider {
  display: flex;
  align-items: center;
  gap: 7px;
  min-height: 34px;
  margin: 14px 10px 0;
  color: #53606b;
  font-size: 12px;
  font-weight: 700;
  border-top: 1px solid #e2e7ed;
}

.editor-panel {
  display: grid;
  min-height: 0;
  grid-template-rows: auto auto minmax(0, 1fr);
}

.run-grid {
  display: grid;
  grid-template-columns: 190px 160px minmax(0, 1fr);
  gap: 12px;
  padding: 14px 16px;
  border-bottom: 1px solid #e7ebf0;
}

.target-input {
  min-width: 180px;
}

.code-editor {
  min-height: 360px;
  padding: 14px 16px;
  color: #f0f4f8;
  background: #171c21;
  border: 0;
  border-radius: 0;
  font-family: "JetBrains Mono", "Cascadia Code", Consolas, monospace;
  font-size: 13px;
  line-height: 1.55;
  white-space: pre;
}

.sessions-panel {
  min-height: 190px;
}

.session-list {
  display: grid;
  gap: 8px;
  padding: 12px;
}

.session-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 36px;
  align-items: center;
  gap: 10px;
  min-height: 46px;
  padding: 0 8px 0 10px;
  background: #f8fafc;
  border: 1px solid #e3e8ee;
  border-radius: 8px;
}

.session-row div {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.session-row strong,
.session-row span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.session-row span {
  color: #6d7985;
  font-size: 12px;
}

.log-console {
  min-height: calc(100vh - 178px);
  padding: 8px 0;
  overflow: auto;
  color: #dce5e8;
  background: #181d20;
  border: 1px solid #30383d;
  border-radius: 8px;
  font-family: "JetBrains Mono", "Cascadia Code", Consolas, monospace;
  font-size: 12px;
}

.log-line {
  display: grid;
  grid-template-columns: 74px 58px 180px minmax(0, 1fr);
  gap: 10px;
  min-height: 28px;
  padding: 5px 12px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.log-line.warn {
  color: #ffd990;
}

.log-line.error {
  color: #ffb4a9;
}

.log-level {
  text-transform: uppercase;
}

.log-source,
.log-message {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.spin {
  animation: spin 0.9s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 960px) {
  .app-shell {
    grid-template-columns: 72px minmax(0, 1fr);
  }

  .sidebar {
    padding: 14px 10px;
  }

  .brand {
    justify-content: center;
  }

  .brand-text,
  .nav-label,
  .sidebar-footer {
    display: none;
  }

  .nav-button {
    grid-template-columns: 1fr;
    justify-items: center;
    padding: 0;
  }

  .topbar,
  .toolbar-band {
    align-items: stretch;
    flex-direction: column;
  }

  .top-actions,
  .toolbar-right {
    justify-content: flex-start;
    flex-wrap: wrap;
  }

  .grid-2,
  .script-layout,
  .run-grid {
    grid-template-columns: 1fr;
  }

  .script-list {
    grid-row: auto;
  }

  .span-2 {
    grid-column: auto;
  }

  .search-box {
    width: 100%;
  }

  .log-line {
    grid-template-columns: 68px 52px minmax(80px, 120px) minmax(0, 1fr);
  }
}
</style>
