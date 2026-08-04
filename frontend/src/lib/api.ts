export interface ToolStatus {
  name: string;
  path: string;
  found: boolean;
  source: string;
  version: string;
  error: string;
}

export interface SystemStatus {
  adb: ToolStatus;
  frida: ToolStatus;
  python: ToolStatus;
  generatedAt: string;
}

export interface Device {
  serial: string;
  state: string;
  model: string;
  product: string;
  transportId: string;
  isAuthorized: boolean;
}

export interface AndroidApp {
  package: string;
  path: string;
  name: string;
  system: boolean;
}

export interface AndroidProcess {
  pid: number;
  user: string;
  name: string;
  package: string;
}

export interface ScriptTemplate {
  id: string;
  name: string;
  category: string;
  description: string;
  source: string;
}

export interface ImportedScript {
  name: string;
  path: string;
  source: string;
}

export interface FridaRuntimeStatus {
  latestVersion: string;
  cacheDir: string;
  host: ToolStatus;
  python: ToolStatus;
  server: ToolStatus;
}

export interface FridaRuntimeRequest {
  deviceSerial: string;
  version: string;
}

export interface FridaRuntimeProgress {
  phase: "release" | "adb" | "python" | "host" | "server" | "runtime" | string;
  stage: string;
  version: string;
  asset: string;
  bytesDownloaded: number;
  totalBytes: number;
  percent: number;
  message: string;
  done: boolean;
  error: string;
}

export interface UpdatePreferences {
  autoCheckAppUpdates: boolean;
  autoCheckFridaUpdates: boolean;
  autoUpdateFrida: boolean;
  notifyWhenUpdateAvailable: boolean;
  autoPreflightBeforeInject: boolean;
}

export interface ComponentUpdateStatus {
  component: "application" | "frida";
  repository: string;
  currentVersion: string;
  latestVersion: string;
  updateAvailable: boolean;
  releaseUrl: string;
  publishedAt: string;
  checkedAt: string;
  error: string;
}

export interface UpdateOverview {
  application: ComponentUpdateStatus;
  frida: ComponentUpdateStatus;
  installDirectory: string;
  fridaDirectory: string;
}

export interface LocalScript {
  id: string;
  name: string;
  description: string;
  tags: string[];
  favorite: boolean;
  source: string;
  origin: string;
  createdAt: string;
  updatedAt: string;
  lastUsedAt: string;
}

export interface SaveLocalScriptRequest {
  id: string;
  name: string;
  description: string;
  tags: string[];
  favorite: boolean;
  source: string;
  origin: string;
}

export interface CodeShareProjectSummary {
  ref: string;
  name: string;
  description: string;
  owner: string;
  slug: string;
  likes: number;
  views: string;
  url: string;
}

export interface CodeShareSearchResult {
  items: CodeShareProjectSummary[];
  query: string;
  page: number;
  totalPages: number;
  source: "online" | "cache";
  cachedAt: string;
  warning: string;
}

export interface CodeShareProject {
  ref: string;
  id: string;
  name: string;
  description: string;
  owner: string;
  slug: string;
  fridaVersion: string;
  likes: number;
  source: string;
  fingerprint: string;
  trustState: "new" | "trusted" | "changed";
  url: string;
  origin: "online" | "cache";
  cachedAt: string;
  warning: string;
}

export interface OperationTemplate {
  id: string;
  name: string;
  category: string;
  description: string;
  requiresDevice: boolean;
}

export interface LogEntry {
  time: string;
  level: "info" | "warn" | "error";
  source: string;
  message: string;
  diagnostic?: DiagnosticFinding;
}

export interface DiagnosticFinding {
  code: string;
  title: string;
  reason: string;
  action: string;
  recovery: "attach" | "server-check" | "processes" | "devices" | string;
  recoverable: boolean;
}

export interface FridaServerStatus {
  state: "unknown" | "running" | "stopped" | "degraded" | "mismatch" | "unavailable";
  pid: string;
  user: string;
  path: string;
  serverVersion: string;
  cliVersion: string;
  versionMatch: boolean;
  checkedAt: string;
  error: string;
}

export interface SessionInfo {
  id: string;
  deviceSerial: string;
  mode: "attach" | "spawn";
  targetKind: "pid" | "name" | "package";
  target: string;
  scriptName: string;
  startedAt: string;
  endedAt?: string;
  durationMs: number;
  exitReason?: string;
  exitCode?: number | null;
  error?: string;
  logCount: number;
  running: boolean;
  stopping: boolean;
}

export interface SessionLogEntry {
  sessionId: string;
  sequence: number;
  timestamp: string;
  stream: "stdout" | "stderr" | "system";
  message: string;
}

export interface RunProfile {
  id: string;
  name: string;
  deviceSerial: string;
  packageName: string;
  mode: "attach" | "spawn";
  targetKind: "pid" | "name" | "package";
  target: string;
  scriptID: string;
  scriptName: string;
  scriptSource: string;
  variables: Record<string, string>;
  serverPath: string;
  createdAt: string;
  updatedAt: string;
  lastRunAt: string;
}

export type SaveRunProfileRequest = Omit<RunProfile, "createdAt" | "updatedAt" | "lastRunAt">;

export interface PreflightRequest {
  deviceSerial: string;
  mode: "attach" | "spawn";
  targetKind: "pid" | "name" | "package";
  target: string;
  serverPath: string;
}

export interface PreflightCheck {
  id: string;
  label: string;
  status: "pass" | "fail" | "warning";
  message: string;
  action: string;
}

export interface PreflightResult {
  ready: boolean;
  deviceSerial: string;
  abi: string;
  cliVersion: string;
  serverVersion: string;
  serverPath: string;
  targetAvailable: boolean;
  checkedAt: string;
  checks: PreflightCheck[];
}

export interface ScriptParameter {
  name: string;
  label: string;
  description: string;
  type: "string" | "number" | "boolean" | "select";
  required: boolean;
  default?: unknown;
  hasDefault: boolean;
  options: string[];
  placeholder: string;
}

export interface ScriptMetadata {
  name: string;
  description: string;
  parameters: ScriptParameter[];
}

export interface FridaServerRequest {
  deviceSerial: string;
  localPath: string;
  remotePath: string;
  forceRestart: boolean;
}

export interface RunScriptRequest {
  deviceSerial: string;
  mode: "attach" | "spawn";
  targetKind: "pid" | "name" | "package";
  target: string;
  scriptName: string;
  scriptSource: string;
  variables: Record<string, string | number | boolean>;
}

export interface RunOperationRequest {
  id: string;
  deviceSerial: string;
}

type BackendMethod = (...args: unknown[]) => Promise<unknown>;
type BackendApp = Record<string, BackendMethod>;

declare global {
  interface Window {
    go?: {
      main?: {
        App?: BackendApp;
      };
    };
    runtime?: {
      EventsOn?: (eventName: string, callback: (...args: unknown[]) => void) => (() => void) | void;
      EventsOff?: (eventName: string) => void;
    };
  }
}

const mockScripts: ScriptTemplate[] = [
  {
    id: "mock",
    name: "示例脚本",
    category: "本地预览",
    description: "Wails 后端未注入时显示的前端预览脚本。",
    source: "console.log('Frida GUI Helper preview');"
  }
];

const mockBackend: BackendApp = {
  async GetSystemStatus() {
    return {
      adb: { name: "adb", path: "", found: false, source: "", version: "", error: "等待 Wails 后端连接" },
      frida: { name: "frida", path: "", found: false, source: "", version: "", error: "等待 Wails 后端连接" },
      python: { name: "python", path: "", found: false, source: "", version: "", error: "等待 Wails 后端连接" },
      generatedAt: new Date().toISOString()
    } satisfies SystemStatus;
  },
  async ListDevices() {
    return [] satisfies Device[];
  },
  async ListApps() {
    return [] satisfies AndroidApp[];
  },
  async ListProcesses() {
    return [] satisfies AndroidProcess[];
  },
  async StartFridaServer() {
    return undefined;
  },
  async GetFridaServerStatus() {
    return {
      state: "unavailable",
      pid: "",
      user: "",
      path: "",
      serverVersion: "",
      cliVersion: "",
      versionMatch: false,
      checkedAt: new Date().toISOString(),
      error: "Wails 后端未连接"
    } satisfies FridaServerStatus;
  },
  async GetFridaRuntimeStatus() {
    return {
      latestVersion: "",
      cacheDir: "",
      host: { name: "frida", path: "", found: false, source: "", version: "", error: "Wails 后端未连接" },
      python: { name: "python", path: "", found: false, source: "", version: "", error: "Wails 后端未连接" },
      server: { name: "frida-server", path: "", found: false, source: "", version: "", error: "Wails 后端未连接" }
    } satisfies FridaRuntimeStatus;
  },
  async PrepareFridaRuntime() {
    return {
      latestVersion: "",
      cacheDir: "",
      host: { name: "frida", path: "", found: false, source: "", version: "", error: "Wails 后端未连接" },
      python: { name: "python", path: "", found: false, source: "", version: "", error: "Wails 后端未连接" },
      server: { name: "frida-server", path: "", found: false, source: "", version: "", error: "Wails 后端未连接" }
    } satisfies FridaRuntimeStatus;
  },
  async GetSettings() {
    return {
      autoCheckAppUpdates: true,
      autoCheckFridaUpdates: true,
      autoUpdateFrida: false,
      notifyWhenUpdateAvailable: true,
      autoPreflightBeforeInject: false
    } satisfies UpdatePreferences;
  },
  async SaveSettings(...args: unknown[]) {
    return args[0] as UpdatePreferences;
  },
  async GetUpdateOverview() {
    const empty = (component: "application" | "frida"): ComponentUpdateStatus => ({
      component,
      repository: component === "application" ? "2218164692/Frida-GUI-Helper" : "frida/frida",
      currentVersion: "",
      latestVersion: "",
      updateAvailable: false,
      releaseUrl: "",
      publishedAt: "",
      checkedAt: "",
      error: "Wails 后端未连接"
    });
    return {
      application: empty("application"),
      frida: empty("frida"),
      installDirectory: "",
      fridaDirectory: ""
    } satisfies UpdateOverview;
  },
  async CheckApplicationUpdate() {
    const overview = (await mockBackend.GetUpdateOverview()) as UpdateOverview;
    return overview.application;
  },
  async CheckFridaUpdate() {
    const overview = (await mockBackend.GetUpdateOverview()) as UpdateOverview;
    return overview.frida;
  },
  async UpdateFridaComponent() {
    return (await mockBackend.PrepareFridaRuntime()) as FridaRuntimeStatus;
  },
  async ExportDiagnosticBundle() {
    return "";
  },
  async ListScripts() {
    return mockScripts;
  },
  async ImportScriptFile() {
    return { name: "", path: "", source: "" } satisfies ImportedScript;
  },
  async ListLocalScripts() {
    return [] satisfies LocalScript[];
  },
  async SaveLocalScript(...args: unknown[]) {
    const request = args[0] as SaveLocalScriptRequest;
    return {
      id: request.id || "preview",
      name: request.name,
      description: request.description,
      tags: request.tags,
      favorite: request.favorite,
      source: request.source,
      origin: request.origin,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
      lastUsedAt: ""
    } satisfies LocalScript;
  },
  async DeleteLocalScript() {
    return undefined;
  },
  async RecordLocalScriptRun() {
    return undefined;
  },
  async SearchCodeShare(...args: unknown[]) {
    const query = typeof args[0] === "string" ? args[0] : "";
    const page = typeof args[1] === "number" ? args[1] : 1;
    return {
      items: [],
      query,
      page,
      totalPages: 1,
      source: "online",
      cachedAt: "",
      warning: "Wails 后端未连接，无法访问 CodeShare"
    } satisfies CodeShareSearchResult;
  },
  async GetCodeShareProject() {
    throw new Error("Wails 后端未连接，无法加载 CodeShare 项目");
  },
  async TrustCodeShareProject() {
    return undefined;
  },
  async ListOperations() {
    return [] satisfies OperationTemplate[];
  },
  async RunOperation() {
    return undefined;
  },
  async ListProfiles() {
    return [] satisfies RunProfile[];
  },
  async SaveProfile(...args: unknown[]) {
    const request = args[0] as SaveRunProfileRequest;
    const now = new Date().toISOString();
    return {
      ...request,
      id: request.id || "preview-profile",
      createdAt: now,
      updatedAt: now,
      lastRunAt: ""
    } satisfies RunProfile;
  },
  async DeleteProfile() {
    return undefined;
  },
  async GetLastProfile() {
    return null;
  },
  async MarkProfileRun(...args: unknown[]) {
    const id = String(args[0] || "preview-profile");
    return {
      id,
      name: "预览配置",
      deviceSerial: "",
      packageName: "",
      mode: "attach",
      targetKind: "name",
      target: "preview",
      scriptID: "mock",
      scriptName: "示例脚本",
      scriptSource: mockScripts[0].source,
      variables: {},
      serverPath: "/data/local/tmp/frida-server",
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
      lastRunAt: new Date().toISOString()
    } satisfies RunProfile;
  },
  async RunPreflight(...args: unknown[]) {
    const request = args[0] as PreflightRequest;
    return {
      ready: false,
      deviceSerial: request.deviceSerial,
      abi: "",
      cliVersion: "",
      serverVersion: "",
      serverPath: request.serverPath,
      targetAvailable: false,
      checkedAt: new Date().toISOString(),
      checks: [{
        id: "backend",
        label: "Wails 后端",
        status: "fail",
        message: "Wails 后端未连接",
        action: "请运行桌面程序"
      }]
    } satisfies PreflightResult;
  },
  async RestartFridaServerAndCheck(...args: unknown[]) {
    return mockBackend.RunPreflight(...args);
  },
  async GetScriptMetadata() {
    return null;
  },
  async RunScript() {
    return {
      id: "preview",
      deviceSerial: "",
      mode: "attach",
      targetKind: "name",
      target: "preview",
      scriptName: "示例脚本",
      startedAt: new Date().toISOString(),
      endedAt: new Date().toISOString(),
      durationMs: 0,
      exitReason: "preview",
      exitCode: 0,
      error: "",
      logCount: 0,
      running: false,
      stopping: false
    } satisfies SessionInfo;
  },
  async StopSession() {
    return undefined;
  },
  async ListSessions() {
    return [] satisfies SessionInfo[];
  },
  async GetSessionLogs() {
    return [] satisfies SessionLogEntry[];
  },
  async RestartSession() {
    return mockBackend.RunScript();
  },
  async GetLogs() {
    return [] satisfies LogEntry[];
  },
  async ClearLogs() {
    return undefined;
  }
};

function backend(): BackendApp {
  return window.go?.main?.App ?? mockBackend;
}

async function invoke<T>(name: string, ...args: unknown[]): Promise<T> {
  const fn = backend()[name] ?? mockBackend[name];
  if (!fn) {
    throw new Error(`后端方法不存在: ${name}`);
  }
  return (await fn(...args)) as T;
}

async function invokeArray<T>(name: string, ...args: unknown[]): Promise<T[]> {
  const value = await invoke<T[] | null | undefined>(name, ...args);
  return Array.isArray(value) ? value : [];
}

export const api = {
  getSystemStatus: () => invoke<SystemStatus>("GetSystemStatus"),
  listDevices: () => invokeArray<Device>("ListDevices"),
  listApps: (serial: string, includeSystem: boolean) => invokeArray<AndroidApp>("ListApps", serial, includeSystem),
  listProcesses: (serial: string) => invokeArray<AndroidProcess>("ListProcesses", serial),
  startFridaServer: (request: FridaServerRequest) => invoke<void>("StartFridaServer", request),
  getFridaServerStatus: (serial: string) => invoke<FridaServerStatus>("GetFridaServerStatus", serial),
  getFridaRuntimeStatus: () => invoke<FridaRuntimeStatus>("GetFridaRuntimeStatus"),
  prepareFridaRuntime: (request: FridaRuntimeRequest) =>
    invoke<FridaRuntimeStatus>("PrepareFridaRuntime", request),
  getSettings: () => invoke<UpdatePreferences>("GetSettings"),
  saveSettings: (settings: UpdatePreferences) => invoke<UpdatePreferences>("SaveSettings", settings),
  getUpdateOverview: () => invoke<UpdateOverview>("GetUpdateOverview"),
  checkApplicationUpdate: () => invoke<ComponentUpdateStatus>("CheckApplicationUpdate"),
  checkFridaUpdate: () => invoke<ComponentUpdateStatus>("CheckFridaUpdate"),
  updateFridaComponent: () => invoke<FridaRuntimeStatus>("UpdateFridaComponent"),
  exportDiagnosticBundle: (serial: string) => invoke<string>("ExportDiagnosticBundle", serial),
  listScripts: () => invokeArray<ScriptTemplate>("ListScripts"),
  importScriptFile: () => invoke<ImportedScript>("ImportScriptFile"),
  listLocalScripts: () => invokeArray<LocalScript>("ListLocalScripts"),
  saveLocalScript: (request: SaveLocalScriptRequest) => invoke<LocalScript>("SaveLocalScript", request),
  deleteLocalScript: (id: string) => invoke<void>("DeleteLocalScript", id),
  recordLocalScriptRun: (id: string) => invoke<void>("RecordLocalScriptRun", id),
  searchCodeShare: (query: string, page: number) =>
    invoke<CodeShareSearchResult>("SearchCodeShare", query, page),
  getCodeShareProject: (projectRef: string) =>
    invoke<CodeShareProject>("GetCodeShareProject", projectRef),
  trustCodeShareProject: (projectRef: string, fingerprint: string) =>
    invoke<void>("TrustCodeShareProject", projectRef, fingerprint),
  listOperations: () => invokeArray<OperationTemplate>("ListOperations"),
  runOperation: (request: RunOperationRequest) => invoke<void>("RunOperation", request),
  listProfiles: () => invokeArray<RunProfile>("ListProfiles"),
  saveProfile: (request: SaveRunProfileRequest) => invoke<RunProfile>("SaveProfile", request),
  deleteProfile: (id: string) => invoke<void>("DeleteProfile", id),
  getLastProfile: () => invoke<RunProfile | null>("GetLastProfile"),
  markProfileRun: (id: string) => invoke<RunProfile>("MarkProfileRun", id),
  runPreflight: (request: PreflightRequest) => invoke<PreflightResult>("RunPreflight", request),
  restartFridaServerAndCheck: (request: PreflightRequest) =>
    invoke<PreflightResult>("RestartFridaServerAndCheck", request),
  getScriptMetadata: (source: string) => invoke<ScriptMetadata | null>("GetScriptMetadata", source),
  runScript: (request: RunScriptRequest) => invoke<SessionInfo>("RunScript", request),
  stopSession: (sessionID: string) => invoke<void>("StopSession", sessionID),
  listSessions: () => invokeArray<SessionInfo>("ListSessions"),
  getSessionLogs: (sessionID: string) => invokeArray<SessionLogEntry>("GetSessionLogs", sessionID),
  restartSession: (sessionID: string) => invoke<SessionInfo>("RestartSession", sessionID),
  getLogs: () => invokeArray<LogEntry>("GetLogs"),
  clearLogs: () => invoke<void>("ClearLogs")
};

export function subscribeLogs(handler: (entry: LogEntry) => void): () => void {
  const off = window.runtime?.EventsOn?.("log:new", (entry) => handler(entry as LogEntry));
  if (typeof off === "function") {
    return off;
  }
  return () => window.runtime?.EventsOff?.("log:new");
}

export function subscribeSessionUpdates(handler: (session: SessionInfo) => void): () => void {
  const off = window.runtime?.EventsOn?.("session:updated", (session) => handler(session as SessionInfo));
  if (typeof off === "function") {
    return off;
  }
  return () => window.runtime?.EventsOff?.("session:updated");
}

export function subscribeSessionLogs(handler: (entry: SessionLogEntry) => void): () => void {
  const off = window.runtime?.EventsOn?.("session:log", (entry) => handler(entry as SessionLogEntry));
  if (typeof off === "function") {
    return off;
  }
  return () => window.runtime?.EventsOff?.("session:log");
}

export function subscribeRuntimeProgress(handler: (progress: FridaRuntimeProgress) => void): () => void {
  const off = window.runtime?.EventsOn?.("runtime:progress", (progress) => handler(progress as FridaRuntimeProgress));
  if (typeof off === "function") {
    return off;
  }
  return () => window.runtime?.EventsOff?.("runtime:progress");
}

export function subscribeUpdateStatus(handler: (status: UpdateOverview) => void): () => void {
  const off = window.runtime?.EventsOn?.("update:status", (status) => handler(status as UpdateOverview));
  if (typeof off === "function") {
    return off;
  }
  return () => window.runtime?.EventsOff?.("update:status");
}

export function subscribeUpdateNotification(handler: (message: string) => void): () => void {
  const off = window.runtime?.EventsOn?.("update:notification", (message) => handler(String(message || "")));
  if (typeof off === "function") {
    return off;
  }
  return () => window.runtime?.EventsOff?.("update:notification");
}
