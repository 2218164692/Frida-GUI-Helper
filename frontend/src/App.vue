<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import {
  AlertTriangle,
  ChevronLeft,
  ChevronRight,
  CheckCircle2,
  Cloud,
  Cpu,
  Database,
  Download,
  FileCode2,
  ListTree,
  MonitorSmartphone,
  Play,
  PlugZap,
  RefreshCw,
  Save,
  Search,
  Send,
  Server,
  ShieldCheck,
  Square,
  Star,
  StarOff,
  Terminal,
  Trash2,
  Upload,
  Wrench,
  Usb
} from "@lucide/vue";
import ScriptEditor from "./components/ScriptEditor.vue";
import {
  api,
  subscribeLogs,
  type AndroidApp,
  type AndroidProcess,
  type CodeShareProject,
  type CodeShareProjectSummary,
  type CodeShareSearchResult,
  type Device,
  type LocalScript,
  type LogEntry,
  type OperationTemplate,
  type SaveLocalScriptRequest,
  type ScriptTemplate,
  type SessionInfo,
  type SystemStatus,
  type ToolStatus
} from "./lib/api";

type ViewKey = "devices" | "processes" | "scripts" | "logs";
type RunMode = "attach" | "spawn";
type TargetKind = "pid" | "name" | "package";
type LevelFilter = "all" | "info" | "warn" | "error";
type ScriptHubTab = "local" | "codeshare";
type ScriptListItem = ScriptTemplate & {
  favorite?: boolean;
  tags?: string[];
  lastUsedAt?: string;
  origin?: string;
};
type ScriptEditorApi = {
  focus: () => void;
  format: () => Promise<void>;
  find: () => void;
  replace: (source: string) => void;
};

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

const scripts = ref<ScriptListItem[]>([]);
const savedScripts = ref<LocalScript[]>([]);
const operations = ref<OperationTemplate[]>([]);
const scriptHubTab = ref<ScriptHubTab>("local");
const selectedScriptID = ref("");
const editorSource = ref("");
const scriptQuery = ref("");
const selectedTag = ref("all");
const scriptNameInput = ref("");
const scriptDescriptionInput = ref("");
const scriptTagsInput = ref("");
const scriptFavorite = ref(false);
const variablePackageName = ref("");
const variableModuleName = ref("");
const variableFunctionName = ref("");
const variableOutputDir = ref("/data/local/tmp");
const codeShareQuery = ref("");
const codeShareResult = ref<CodeShareSearchResult | null>(null);
const activeCodeShareProject = ref<CodeShareProject | null>(null);
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
let scriptEditor: ScriptEditorApi | null = null;

const currentTitle = computed(() => views.find((view) => view.key === activeView.value)?.label ?? "");
const selectedDevice = computed(() => devices.value.find((device) => device.serial === selectedSerial.value));
const selectedDeviceLabel = computed(() => {
  if (!selectedDevice.value) {
    return "未选择设备";
  }
  const model = selectedDevice.value.model || selectedDevice.value.product || selectedDevice.value.state;
  return `${selectedDevice.value.serial}${model ? ` · ${model}` : ""}`;
});

const activeScript = computed(() =>
  [...savedScriptItems.value, ...scripts.value].find((script) => script.id === selectedScriptID.value)
);
const activeScriptName = computed(() => activeScript.value?.name || "自定义脚本");
const canRunScript = computed(() => Boolean(selectedSerial.value && target.value.trim() && editorSource.value.trim()));
const savedScriptItems = computed<ScriptListItem[]>(() =>
  savedScripts.value.map((script) => ({
    id: `saved:${script.id}`,
    name: script.name,
    category: script.favorite ? "我的收藏" : "我的脚本",
    description: script.description || script.origin || "本地脚本库",
    source: script.source,
    favorite: script.favorite,
    tags: script.tags,
    lastUsedAt: script.lastUsedAt,
    origin: script.origin
  }))
);
const localScripts = computed(() => {
  const query = scriptQuery.value.trim().toLowerCase();
  const tag = selectedTag.value;
  return [...savedScriptItems.value, ...scripts.value.filter((script) => !script.id.startsWith("codeshare:"))].filter(
    (script) => {
      const tags = script.tags ?? [];
      const tagMatches = tag === "all" || tags.some((item) => item.toLowerCase() === tag.toLowerCase());
      const textMatches =
        !query ||
        [script.name, script.category, script.description, tags.join(" ")].some((value) =>
          String(value).toLowerCase().includes(query)
        );
      return tagMatches && textMatches;
    }
  );
});
const localScriptTags = computed(() => {
  const tags = new Set<string>();
  savedScripts.value.forEach((script) => script.tags.forEach((tag) => tags.add(tag)));
  return Array.from(tags).sort((a, b) => a.localeCompare(b));
});
const codeShareItems = computed(() => codeShareResult.value?.items ?? []);
const codeShareSourceLabel = computed(() => codeShareResult.value?.source === "cache" ? "本地缓存" : "在线");
const selectedLocalScriptID = computed(() =>
  selectedScriptID.value.startsWith("saved:") ? selectedScriptID.value.slice("saved:".length) : ""
);

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
    const externalScripts = scripts.value.filter((script) =>
      script.id.startsWith("imported:") || script.id.startsWith("codeshare:")
    );
    const [builtInScripts, localLibrary] = await Promise.all([api.listScripts(), api.listLocalScripts()]);
    savedScripts.value = localLibrary;
    scripts.value = [...externalScripts, ...builtInScripts];
    if (!selectedScriptID.value && scripts.value.length > 0) {
      selectScript(savedScriptItems.value[0] ?? scripts.value[0]);
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
    scriptHubTab.value = "local";
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
  scriptNameInput.value = script.name || "";
  scriptDescriptionInput.value = script.description || "";
  scriptTagsInput.value = (("tags" in script && Array.isArray(script.tags)) ? script.tags : []).join(", ");
  scriptFavorite.value = Boolean("favorite" in script && script.favorite);
  if (script.id.startsWith("codeshare:")) {
    scriptHubTab.value = "codeshare";
  } else {
    scriptHubTab.value = "local";
    activeCodeShareProject.value = null;
  }
  activeView.value = "scripts";
}

function onScriptEditorReady(apiRef: ScriptEditorApi) {
  scriptEditor = apiRef;
}

async function saveCurrentScript() {
  await withBusy("save-script", async () => {
    const request: SaveLocalScriptRequest = {
      id: selectedLocalScriptID.value,
      name: scriptNameInput.value.trim() || activeScriptName.value,
      description: scriptDescriptionInput.value.trim(),
      tags: parseTags(scriptTagsInput.value),
      favorite: scriptFavorite.value,
      source: editorSource.value,
      origin: activeCodeShareProject.value ? `codeshare:@${activeCodeShareProject.value.ref}` : "local"
    };
    const saved = await api.saveLocalScript(request);
    await refreshScripts();
    const item = savedScriptItems.value.find((script) => script.id === `saved:${saved.id}`);
    if (item) {
      selectScript(item);
    }
  });
}

async function deleteCurrentLocalScript() {
  if (!selectedLocalScriptID.value) {
    return;
  }
  if (!window.confirm(`确认删除本地脚本“${activeScriptName.value}”？`)) {
    return;
  }
  await withBusy("delete-script", async () => {
    await api.deleteLocalScript(selectedLocalScriptID.value);
    selectedScriptID.value = "";
    await refreshScripts();
    const fallback = savedScriptItems.value[0] ?? scripts.value.find((script) => !script.id.startsWith("codeshare:"));
    if (fallback) {
      selectScript(fallback);
    }
  });
}

async function toggleCurrentFavorite() {
  scriptFavorite.value = !scriptFavorite.value;
  if (selectedLocalScriptID.value) {
    await saveCurrentScript();
  }
}

function parseTags(value: string) {
  return value
    .split(/[,，\s]+/)
    .map((tag) => tag.trim().replace(/^#/, ""))
    .filter(Boolean)
    .slice(0, 12);
}

function applyRuntimeVariables(source: string) {
  const values: Record<string, string> = {
    deviceSerial: selectedSerial.value,
    target: target.value.trim(),
    targetKind: runMode.value === "spawn" ? "package" : targetKind.value,
    packageName:
      variablePackageName.value.trim() ||
      (runMode.value === "spawn" || targetKind.value === "package" ? target.value.trim() : ""),
    moduleName: variableModuleName.value.trim(),
    functionName: variableFunctionName.value.trim(),
    outputDir: variableOutputDir.value.trim() || "/data/local/tmp"
  };
  return source.replace(/\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}/g, (match, key: string) =>
    Object.prototype.hasOwnProperty.call(values, key) ? values[key] : match
  );
}

function applyVariablesToEditor() {
  const nextSource = applyRuntimeVariables(editorSource.value);
  editorSource.value = nextSource;
  scriptEditor?.replace(nextSource);
  appendLocalLog("info", "ui", "已将运行变量写入编辑器");
}

async function openCodeShare() {
  scriptHubTab.value = "codeshare";
  if (!codeShareResult.value) {
    await searchCodeShare(1);
  }
}

async function searchCodeShare(page = 1) {
  await withBusy("codeshare-search", async () => {
    const result = await api.searchCodeShare(codeShareQuery.value.trim(), page);
    result.items = Array.isArray(result.items) ? result.items : [];
    result.page = Math.max(1, result.page || page);
    result.totalPages = Math.max(1, result.totalPages || 1);
    codeShareResult.value = result;
    if (result.warning) {
      appendLocalLog("warn", "codeshare", result.warning);
    }
  });
}

async function loadCodeShareProject(summary: CodeShareProjectSummary) {
  await withBusy(`codeshare-project:${summary.ref}`, async () => {
    const project = await api.getCodeShareProject(summary.ref);
    if (project.warning) {
      appendLocalLog("warn", "codeshare", project.warning);
    }

    if (project.trustState !== "trusted") {
      const changed = project.trustState === "changed";
      const origin = project.origin === "cache"
        ? `当前源码来自 ${formatDateTime(project.cachedAt)} 的本地缓存。`
        : "当前源码来自 Frida CodeShare 在线服务。";
      const message = [
        changed ? "该项目的源码已发生变化，需要重新确认。" : "这是首次载入该 CodeShare 项目。",
        `项目：${project.name}`,
        `作者：@${project.owner}`,
        `SHA-256：${project.fingerprint}`,
        origin,
        "CodeShare 是社区脚本库。确认信任前请检查源码，仅在已授权目标上运行。",
        "",
        "是否信任当前指纹并载入编辑器？"
      ].join("\n");
      if (!window.confirm(message)) {
        appendLocalLog("warn", "codeshare", `已取消信任 @${project.ref}`);
        return;
      }
      await api.trustCodeShareProject(project.ref, project.fingerprint);
      project.trustState = "trusted";
    }

    const script: ScriptTemplate = {
      id: `codeshare:${project.ref}:${project.fingerprint}`,
      name: project.name,
      category: "CodeShare",
      description: `@${project.owner} · ${project.description || "Frida CodeShare community script"}`,
      source: project.source
    };
    scripts.value = [script, ...scripts.value.filter((item) => !item.id.startsWith(`codeshare:${project.ref}:`))];
    activeCodeShareProject.value = project;
    selectScript(script);
    appendLocalLog("info", "codeshare", `已载入 @${project.ref}，SHA-256: ${project.fingerprint}`);
  });
}

function previousCodeSharePage() {
  const page = codeShareResult.value?.page ?? 1;
  if (page > 1) {
    void searchCodeShare(page - 1);
  }
}

function nextCodeSharePage() {
  const page = codeShareResult.value?.page ?? 1;
  const totalPages = codeShareResult.value?.totalPages ?? 1;
  if (page < totalPages) {
    void searchCodeShare(page + 1);
  }
}

function formatDateTime(value: string) {
  if (!value) {
    return "未知时间";
  }
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString("zh-CN", { hour12: false });
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
  variablePackageName.value = process.package || process.name || variablePackageName.value;
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
  variablePackageName.value = app.package;
  activeView.value = "scripts";
}

async function spawnApp(app: AndroidApp) {
  prepareSpawn(app);
  await runCurrentScript();
}

async function runCurrentScript() {
  await withBusy("run-script", async () => {
    const source = applyRuntimeVariables(editorSource.value);
    const session = await api.runScript({
      deviceSerial: selectedSerial.value,
      mode: runMode.value,
      targetKind: runMode.value === "spawn" ? "package" : targetKind.value,
      target: target.value.trim(),
      scriptName: activeScriptName.value,
      scriptSource: source
    });
    if (selectedLocalScriptID.value) {
      await api.recordLocalScriptRun(selectedLocalScriptID.value);
      await refreshScripts();
    }
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
              <span>设备目标（文件或目录）</span>
              <input v-model="fridaRemotePath" type="text" placeholder="/data/local/tmp/frida-server" />
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
              <p v-if="scriptHubTab === 'local'">{{ localScripts.length }} 个本地脚本</p>
              <p v-else>{{ codeShareItems.length }} 个 CodeShare 项目</p>
            </div>
            <div class="header-controls">
              <button v-if="scriptHubTab === 'local'" class="icon-button" type="button" title="导入脚本" @click="importScriptFile">
                <Upload :size="17" />
              </button>
              <button
                class="icon-button"
                type="button"
                :title="scriptHubTab === 'local' ? '刷新本地脚本' : '刷新 CodeShare'"
                @click="scriptHubTab === 'local' ? refreshScripts() : searchCodeShare(codeShareResult?.page || 1)"
              >
                <RefreshCw :size="17" />
              </button>
            </div>
          </div>

          <div class="hub-tabs segmented wide">
            <button type="button" :class="{ active: scriptHubTab === 'local' }" @click="scriptHubTab = 'local'">
              本地
            </button>
            <button type="button" :class="{ active: scriptHubTab === 'codeshare' }" @click="openCodeShare">
              CodeShare
            </button>
          </div>

          <template v-if="scriptHubTab === 'local'">
            <label class="search-box compact-search script-filter">
              <Search :size="16" />
              <input v-model="scriptQuery" type="search" placeholder="搜索脚本、标签" />
            </label>

            <select v-if="localScriptTags.length > 0" v-model="selectedTag" class="tag-filter">
              <option value="all">全部标签</option>
              <option v-for="tag in localScriptTags" :key="tag" :value="tag">{{ tag }}</option>
            </select>

            <div v-if="localScripts.length === 0" class="empty compact-empty">暂无脚本</div>

            <button
              v-for="script in localScripts"
              :key="script.id"
              class="script-item"
              :class="{ active: selectedScriptID === script.id }"
              type="button"
              @click="selectScript(script)"
            >
              <span class="script-item-meta">
                <Star v-if="script.favorite" :size="12" />
                <span>{{ script.category }}</span>
              </span>
              <strong>{{ script.name }}</strong>
              <small>{{ script.description }}</small>
              <div v-if="script.tags?.length" class="tag-row">
                <span v-for="tag in script.tags" :key="tag">#{{ tag }}</span>
              </div>
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
          </template>

          <template v-else>
            <form class="codeshare-search" @submit.prevent="searchCodeShare(1)">
              <label class="search-box compact-search">
                <Search :size="16" />
                <input v-model="codeShareQuery" type="search" maxlength="120" placeholder="搜索官方社区脚本" />
              </label>
              <button class="icon-button strong" type="submit" title="搜索 CodeShare" :disabled="busy === 'codeshare-search'">
                <Search :size="17" />
              </button>
            </form>

            <div v-if="codeShareResult" class="codeshare-source" :class="codeShareResult.source">
              <Database v-if="codeShareResult.source === 'cache'" :size="14" />
              <Cloud v-else :size="14" />
              <span>{{ codeShareSourceLabel }}</span>
              <small v-if="codeShareResult.cachedAt">{{ formatDateTime(codeShareResult.cachedAt) }}</small>
            </div>

            <div v-if="codeShareResult?.warning" class="codeshare-warning">
              <AlertTriangle :size="15" />
              <span>{{ codeShareResult.warning }}</span>
            </div>

            <div v-if="busy === 'codeshare-search' && !codeShareResult" class="empty compact-empty">正在连接 CodeShare...</div>
            <div v-else-if="codeShareResult && codeShareItems.length === 0" class="empty compact-empty">没有匹配的项目</div>

            <button
              v-for="project in codeShareItems"
              :key="project.ref"
              class="script-item codeshare-item"
              :class="{ active: activeCodeShareProject?.ref === project.ref }"
              type="button"
              :disabled="busy === `codeshare-project:${project.ref}`"
              @click="loadCodeShareProject(project)"
            >
              <span>@{{ project.owner }} · {{ project.likes }} 赞 · {{ project.views || '-' }} 浏览</span>
              <strong>{{ project.name }}</strong>
              <small>{{ project.description || project.ref }}</small>
            </button>

            <div v-if="codeShareResult" class="codeshare-pagination">
              <button class="icon-button" type="button" title="上一页" :disabled="codeShareResult.page <= 1" @click="previousCodeSharePage">
                <ChevronLeft :size="17" />
              </button>
              <span>{{ codeShareResult.page }} / {{ codeShareResult.totalPages }}</span>
              <button class="icon-button" type="button" title="下一页" :disabled="codeShareResult.page >= codeShareResult.totalPages" @click="nextCodeSharePage">
                <ChevronRight :size="17" />
              </button>
            </div>
          </template>
        </div>

        <div class="panel editor-panel" :class="{ 'codeshare-active': activeCodeShareProject }">
          <div class="panel-header">
            <div>
              <h2>脚本编辑器</h2>
              <p>{{ activeScriptName }}</p>
            </div>
            <div class="header-controls">
              <button class="icon-button" type="button" title="收藏脚本" @click="toggleCurrentFavorite">
                <Star v-if="scriptFavorite" :size="17" />
                <StarOff v-else :size="17" />
              </button>
              <button class="icon-button" type="button" title="保存到本地脚本库" @click="saveCurrentScript">
                <Save :size="17" />
              </button>
              <button
                class="icon-button danger"
                type="button"
                title="删除本地脚本"
                :disabled="!selectedLocalScriptID"
                @click="deleteCurrentLocalScript"
              >
                <Trash2 :size="17" />
              </button>
              <button class="icon-button" type="button" title="查找" @click="scriptEditor?.find()">
                <Search :size="17" />
              </button>
              <button class="small-button" type="button" title="格式化脚本" @click="scriptEditor?.format()">格式化</button>
              <button class="primary-button" type="button" :disabled="!canRunScript" @click="runCurrentScript">
                <Play :size="17" />
                <span>注入</span>
              </button>
            </div>
          </div>

          <div v-if="activeCodeShareProject" class="codeshare-meta">
            <div>
              <strong>@{{ activeCodeShareProject.owner }}</strong>
              <span>{{ activeCodeShareProject.likes }} 赞</span>
              <span v-if="activeCodeShareProject.fridaVersion">发布时 Frida {{ activeCodeShareProject.fridaVersion }}</span>
              <span class="source-badge" :class="activeCodeShareProject.origin">
                {{ activeCodeShareProject.origin === 'cache' ? '缓存源码' : '在线源码' }}
              </span>
            </div>
            <code :title="activeCodeShareProject.fingerprint">SHA-256 {{ activeCodeShareProject.fingerprint }}</code>
            <p v-if="activeCodeShareProject.warning">
              <AlertTriangle :size="14" />
              <span>{{ activeCodeShareProject.warning }}</span>
            </p>
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

          <div class="script-meta-grid">
            <label>
              <span>脚本名称</span>
              <input v-model="scriptNameInput" type="text" maxlength="80" placeholder="脚本名称" />
            </label>
            <label>
              <span>标签</span>
              <input v-model="scriptTagsInput" type="text" placeholder="ssl, native, debug" />
            </label>
            <label class="script-description-input">
              <span>描述</span>
              <input v-model="scriptDescriptionInput" type="text" maxlength="180" placeholder="用途说明" />
            </label>
          </div>

          <div class="variable-panel">
            <label>
              <span>packageName</span>
              <input v-model="variablePackageName" type="text" placeholder="{{packageName}}" />
            </label>
            <label>
              <span>moduleName</span>
              <input v-model="variableModuleName" type="text" placeholder="{{moduleName}}" />
            </label>
            <label>
              <span>functionName</span>
              <input v-model="variableFunctionName" type="text" placeholder="{{functionName}}" />
            </label>
            <label>
              <span>outputDir</span>
              <input v-model="variableOutputDir" type="text" placeholder="{{outputDir}}" />
            </label>
            <button class="small-button strong" type="button" @click="applyVariablesToEditor">写入变量</button>
          </div>

          <ScriptEditor v-model="editorSource" class="monaco-shell" language="javascript" @ready="onScriptEditorReady" />
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

.hub-tabs {
  flex: 0 0 auto;
  width: calc(100% - 20px);
  margin: 10px 10px 0;
}

.script-filter,
.tag-filter {
  width: calc(100% - 20px);
  margin: 10px 10px 0;
}

.tag-filter {
  height: 34px;
}

.codeshare-search {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 36px;
  gap: 8px;
  margin: 10px 10px 0;
}

.compact-search {
  width: 100%;
}

.codeshare-source {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 28px;
  margin: 10px 10px 0;
  padding: 0 9px;
  color: #14624f;
  background: #eff9f5;
  border: 1px solid #c8e8dc;
  border-radius: 7px;
  font-size: 12px;
}

.codeshare-source.cache {
  color: #7b4c12;
  background: #fff8e9;
  border-color: #efdcae;
}

.codeshare-source small {
  min-width: 0;
  margin-left: auto;
  overflow: hidden;
  color: inherit;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.codeshare-warning {
  display: grid;
  grid-template-columns: 16px minmax(0, 1fr);
  gap: 7px;
  margin: 8px 10px 0;
  padding: 8px 9px;
  color: #8a321f;
  background: #fff7f3;
  border: 1px solid #ffd8ca;
  border-radius: 7px;
  font-size: 11px;
  line-height: 1.45;
}

.codeshare-warning svg {
  margin-top: 1px;
}

.codeshare-pagination {
  display: grid;
  grid-template-columns: 36px minmax(70px, 1fr) 36px;
  align-items: center;
  gap: 8px;
  margin: 10px;
}

.codeshare-pagination span {
  color: #65727e;
  font-size: 12px;
  text-align: center;
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

.script-item-meta {
  display: inline-flex;
  align-items: center;
  gap: 5px;
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

.tag-row {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  min-width: 0;
}

.tag-row span {
  min-height: 20px;
  padding: 2px 6px;
  color: #14624f;
  background: #e9f7f1;
  border-radius: 5px;
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
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  grid-template-rows: auto auto auto auto auto;
}

.editor-panel.codeshare-active {
  grid-template-rows: auto auto auto auto auto auto;
}

.codeshare-meta {
  display: grid;
  gap: 7px;
  min-width: 0;
  padding: 10px 16px;
  background: #f8fafc;
  border-bottom: 1px solid #e7ebf0;
}

.codeshare-meta > div {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  color: #5d6a76;
  font-size: 12px;
}

.codeshare-meta strong {
  color: #20252d;
}

.codeshare-meta code {
  min-width: 0;
  overflow: hidden;
  color: #53606b;
  font-family: "JetBrains Mono", "Cascadia Code", Consolas, monospace;
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.codeshare-meta p {
  display: grid;
  grid-template-columns: 15px minmax(0, 1fr);
  gap: 7px;
  margin: 0;
  color: #8a321f;
  font-size: 11px;
  line-height: 1.45;
}

.source-badge {
  display: inline-flex;
  align-items: center;
  min-height: 22px;
  padding: 0 7px;
  color: #14624f;
  background: #e9f7f1;
  border-radius: 5px;
}

.source-badge.cache {
  color: #7b4c12;
  background: #fff2d3;
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

.script-meta-grid {
  display: grid;
  grid-template-columns: minmax(180px, 240px) minmax(180px, 240px) minmax(0, 1fr);
  gap: 12px;
  padding: 12px 16px;
  background: #fbfcfd;
  border-bottom: 1px solid #e7ebf0;
}

.script-description-input {
  min-width: 220px;
}

.variable-panel {
  display: grid;
  grid-template-columns: repeat(4, minmax(130px, 1fr)) auto;
  align-items: end;
  gap: 10px;
  padding: 12px 16px;
  background: #f6f8fa;
  border-bottom: 1px solid #e7ebf0;
}

.monaco-shell {
  min-height: 300px;
  overflow: hidden;
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
  .run-grid,
  .script-meta-grid,
  .variable-panel {
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
