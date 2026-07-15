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
}

export interface SessionInfo {
  id: string;
  deviceSerial: string;
  mode: "attach" | "spawn";
  targetKind: "pid" | "name" | "package";
  target: string;
  scriptName: string;
  startedAt: string;
  running: boolean;
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
  async RunScript() {
    return {
      id: "preview",
      deviceSerial: "",
      mode: "attach",
      targetKind: "name",
      target: "preview",
      scriptName: "示例脚本",
      startedAt: new Date().toISOString(),
      running: false
    } satisfies SessionInfo;
  },
  async StopSession() {
    return undefined;
  },
  async ListSessions() {
    return [] satisfies SessionInfo[];
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
  runScript: (request: RunScriptRequest) => invoke<SessionInfo>("RunScript", request),
  stopSession: (sessionID: string) => invoke<void>("StopSession", sessionID),
  listSessions: () => invokeArray<SessionInfo>("ListSessions"),
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
