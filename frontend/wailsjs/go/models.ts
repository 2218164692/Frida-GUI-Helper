export namespace adb {
	
	export class AndroidApp {
	    package: string;
	    path: string;
	    name: string;
	    system: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AndroidApp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.package = source["package"];
	        this.path = source["path"];
	        this.name = source["name"];
	        this.system = source["system"];
	    }
	}
	export class Device {
	    serial: string;
	    state: string;
	    model: string;
	    product: string;
	    transportId: string;
	    isAuthorized: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Device(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serial = source["serial"];
	        this.state = source["state"];
	        this.model = source["model"];
	        this.product = source["product"];
	        this.transportId = source["transportId"];
	        this.isAuthorized = source["isAuthorized"];
	    }
	}
	export class Process {
	    pid: number;
	    user: string;
	    name: string;
	    package: string;
	
	    static createFrom(source: any = {}) {
	        return new Process(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pid = source["pid"];
	        this.user = source["user"];
	        this.name = source["name"];
	        this.package = source["package"];
	    }
	}
	export class ToolStatus {
	    name: string;
	    path: string;
	    found: boolean;
	    source: string;
	    version: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new ToolStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.found = source["found"];
	        this.source = source["source"];
	        this.version = source["version"];
	        this.error = source["error"];
	    }
	}

}

export namespace codeshare {
	
	export class Project {
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
	    trustState: string;
	    url: string;
	    origin: string;
	    cachedAt: string;
	    warning: string;
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.owner = source["owner"];
	        this.slug = source["slug"];
	        this.fridaVersion = source["fridaVersion"];
	        this.likes = source["likes"];
	        this.source = source["source"];
	        this.fingerprint = source["fingerprint"];
	        this.trustState = source["trustState"];
	        this.url = source["url"];
	        this.origin = source["origin"];
	        this.cachedAt = source["cachedAt"];
	        this.warning = source["warning"];
	    }
	}
	export class ProjectSummary {
	    ref: string;
	    name: string;
	    description: string;
	    owner: string;
	    slug: string;
	    likes: number;
	    views: string;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new ProjectSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.owner = source["owner"];
	        this.slug = source["slug"];
	        this.likes = source["likes"];
	        this.views = source["views"];
	        this.url = source["url"];
	    }
	}
	export class SearchResult {
	    items: ProjectSummary[];
	    query: string;
	    page: number;
	    totalPages: number;
	    source: string;
	    cachedAt: string;
	    warning: string;
	
	    static createFrom(source: any = {}) {
	        return new SearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], ProjectSummary);
	        this.query = source["query"];
	        this.page = source["page"];
	        this.totalPages = source["totalPages"];
	        this.source = source["source"];
	        this.cachedAt = source["cachedAt"];
	        this.warning = source["warning"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace frida {
	
	export class SessionInfo {
	    id: string;
	    deviceSerial: string;
	    mode: string;
	    targetKind: string;
	    target: string;
	    scriptName: string;
	    startedAt: string;
	    running: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.deviceSerial = source["deviceSerial"];
	        this.mode = source["mode"];
	        this.targetKind = source["targetKind"];
	        this.target = source["target"];
	        this.scriptName = source["scriptName"];
	        this.startedAt = source["startedAt"];
	        this.running = source["running"];
	    }
	}
	export class ToolStatus {
	    name: string;
	    path: string;
	    found: boolean;
	    source: string;
	    version: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new ToolStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.found = source["found"];
	        this.source = source["source"];
	        this.version = source["version"];
	        this.error = source["error"];
	    }
	}

}

export namespace logstream {
	
	export class Entry {
	    time: string;
	    level: string;
	    source: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = source["time"];
	        this.level = source["level"];
	        this.source = source["source"];
	        this.message = source["message"];
	    }
	}

}

export namespace main {
	
	export class FridaServerRequest {
	    deviceSerial: string;
	    localPath: string;
	    remotePath: string;
	    forceRestart: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FridaServerRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.deviceSerial = source["deviceSerial"];
	        this.localPath = source["localPath"];
	        this.remotePath = source["remotePath"];
	        this.forceRestart = source["forceRestart"];
	    }
	}
	export class ImportedScript {
	    name: string;
	    path: string;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportedScript(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.source = source["source"];
	    }
	}
	export class RunOperationRequest {
	    id: string;
	    deviceSerial: string;
	
	    static createFrom(source: any = {}) {
	        return new RunOperationRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.deviceSerial = source["deviceSerial"];
	    }
	}
	export class RunScriptRequest {
	    deviceSerial: string;
	    mode: string;
	    targetKind: string;
	    target: string;
	    scriptName: string;
	    scriptSource: string;
	
	    static createFrom(source: any = {}) {
	        return new RunScriptRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.deviceSerial = source["deviceSerial"];
	        this.mode = source["mode"];
	        this.targetKind = source["targetKind"];
	        this.target = source["target"];
	        this.scriptName = source["scriptName"];
	        this.scriptSource = source["scriptSource"];
	    }
	}
	export class ToolStatus {
	    name: string;
	    path: string;
	    found: boolean;
	    source: string;
	    version: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new ToolStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.found = source["found"];
	        this.source = source["source"];
	        this.version = source["version"];
	        this.error = source["error"];
	    }
	}
	export class SystemStatus {
	    adb: adb.ToolStatus;
	    frida: frida.ToolStatus;
	    python: ToolStatus;
	    generatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new SystemStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.adb = this.convertValues(source["adb"], adb.ToolStatus);
	        this.frida = this.convertValues(source["frida"], frida.ToolStatus);
	        this.python = this.convertValues(source["python"], ToolStatus);
	        this.generatedAt = source["generatedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace operations {
	
	export class Template {
	    id: string;
	    name: string;
	    category: string;
	    description: string;
	    requiresDevice: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Template(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.category = source["category"];
	        this.description = source["description"];
	        this.requiresDevice = source["requiresDevice"];
	    }
	}

}

export namespace scripts {
	
	export class Template {
	    id: string;
	    name: string;
	    category: string;
	    description: string;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new Template(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.category = source["category"];
	        this.description = source["description"];
	        this.source = source["source"];
	    }
	}

}

