export namespace backend {
	
	export class Component {
	    id: string;
	    kind: string;
	    name: string;
	    display: string;
	    command: string;
	    exePath: string;
	    source: string;
	    trigger: string;
	    startType: string;
	    enabled: boolean;
	    autoStart: boolean;
	    running: boolean;
	    pids: number[];
	    ramBytes: number;
	    ramFormatted: string;
	    sha256: string;
	    virusTotalURL: string;
	    lastRun: string;
	    publisher: string;
	    product: string;
	    signature: string;
	    signer: string;
	    locked: string;
	    system: boolean;
	    review: boolean;
	    flags: string[];
	    control: string;
	    controlNote: string;
	    caution: string;
	    canStop: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Component(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.display = source["display"];
	        this.command = source["command"];
	        this.exePath = source["exePath"];
	        this.source = source["source"];
	        this.trigger = source["trigger"];
	        this.startType = source["startType"];
	        this.enabled = source["enabled"];
	        this.autoStart = source["autoStart"];
	        this.running = source["running"];
	        this.pids = source["pids"];
	        this.ramBytes = source["ramBytes"];
	        this.ramFormatted = source["ramFormatted"];
	        this.sha256 = source["sha256"];
	        this.virusTotalURL = source["virusTotalURL"];
	        this.lastRun = source["lastRun"];
	        this.publisher = source["publisher"];
	        this.product = source["product"];
	        this.signature = source["signature"];
	        this.signer = source["signer"];
	        this.locked = source["locked"];
	        this.system = source["system"];
	        this.review = source["review"];
	        this.flags = source["flags"];
	        this.control = source["control"];
	        this.controlNote = source["controlNote"];
	        this.caution = source["caution"];
	        this.canStop = source["canStop"];
	    }
	}
	export class AppGroup {
	    id: string;
	    name: string;
	    publisher: string;
	    signer: string;
	    letter: string;
	    signature: string;
	    path: string;
	    components: Component[];
	    running: boolean;
	    ramBytes: number;
	    ramFormatted: string;
	    autoStart: boolean;
	    review: boolean;
	    system: boolean;
	    behavior: string;
	    behaviorNote: string;
	
	    static createFrom(source: any = {}) {
	        return new AppGroup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.publisher = source["publisher"];
	        this.signer = source["signer"];
	        this.letter = source["letter"];
	        this.signature = source["signature"];
	        this.path = source["path"];
	        this.components = this.convertValues(source["components"], Component);
	        this.running = source["running"];
	        this.ramBytes = source["ramBytes"];
	        this.ramFormatted = source["ramFormatted"];
	        this.autoStart = source["autoStart"];
	        this.review = source["review"];
	        this.system = source["system"];
	        this.behavior = source["behavior"];
	        this.behaviorNote = source["behaviorNote"];
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
	export class BatchResult {
	    batchId?: string;
	    total: number;
	    modified: number;
	    failed: number;
	    message: string;
	    entries: string[];
	
	    static createFrom(source: any = {}) {
	        return new BatchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.batchId = source["batchId"];
	        this.total = source["total"];
	        this.modified = source["modified"];
	        this.failed = source["failed"];
	        this.message = source["message"];
	        this.entries = source["entries"];
	    }
	}
	export class BootDegradationItem {
	    eventId: number;
	    type: string;
	    name: string;
	    path: string;
	    totalTimeMs: number;
	    delayMs: number;
	    formattedDelay: string;
	
	    static createFrom(source: any = {}) {
	        return new BootDegradationItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.eventId = source["eventId"];
	        this.type = source["type"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.totalTimeMs = source["totalTimeMs"];
	        this.delayMs = source["delayMs"];
	        this.formattedDelay = source["formattedDelay"];
	    }
	}
	export class BootProfile {
	    available: boolean;
	    requiresAdmin: boolean;
	    bootDurationMs: number;
	    mainPathMs: number;
	    bootFormatted: string;
	    bootTime: string;
	    degradations: BootDegradationItem[];
	    summary: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new BootProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.requiresAdmin = source["requiresAdmin"];
	        this.bootDurationMs = source["bootDurationMs"];
	        this.mainPathMs = source["mainPathMs"];
	        this.bootFormatted = source["bootFormatted"];
	        this.bootTime = source["bootTime"];
	        this.degradations = this.convertValues(source["degradations"], BootDegradationItem);
	        this.summary = source["summary"];
	        this.error = source["error"];
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
	export class ChangeRequest {
	    id: string;
	    action: string;
	
	    static createFrom(source: any = {}) {
	        return new ChangeRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.action = source["action"];
	    }
	}
	export class Target {
	    kind: string;
	    root: string;
	    path: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new Target(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.root = source["root"];
	        this.path = source["path"];
	        this.name = source["name"];
	    }
	}
	export class HistoryEntry {
	    id: string;
	    batchId?: string;
	    time: string;
	    title: string;
	    appName: string;
	    kind: string;
	    action: string;
	    summary: string;
	    target: Target;
	    before: string;
	    after: string;
	    beforeRaw: string;
	    beforeExisted: boolean;
	    status: string;
	    restorable: boolean;
	    restoredAt: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new HistoryEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.batchId = source["batchId"];
	        this.time = source["time"];
	        this.title = source["title"];
	        this.appName = source["appName"];
	        this.kind = source["kind"];
	        this.action = source["action"];
	        this.summary = source["summary"];
	        this.target = this.convertValues(source["target"], Target);
	        this.before = source["before"];
	        this.after = source["after"];
	        this.beforeRaw = source["beforeRaw"];
	        this.beforeExisted = source["beforeExisted"];
	        this.status = source["status"];
	        this.restorable = source["restorable"];
	        this.restoredAt = source["restoredAt"];
	        this.error = source["error"];
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
	export class ChangeResult {
	    ok: boolean;
	    message: string;
	    entry?: HistoryEntry;
	
	    static createFrom(source: any = {}) {
	        return new ChangeResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.message = source["message"];
	        this.entry = this.convertValues(source["entry"], HistoryEntry);
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
	
	export class ComponentDiff {
	    component: Component;
	    beforeState: string;
	    afterState: string;
	
	    static createFrom(source: any = {}) {
	        return new ComponentDiff(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.component = this.convertValues(source["component"], Component);
	        this.beforeState = source["beforeState"];
	        this.afterState = source["afterState"];
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
	
	export class SourceInfo {
	    id: string;
	    title: string;
	    desc: string;
	    count: number;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new SourceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.desc = source["desc"];
	        this.count = source["count"];
	        this.status = source["status"];
	    }
	}
	export class Inventory {
	    apps: AppGroup[];
	    sources: SourceInfo[];
	    warnings: string[];
	    scannedAt: string;
	    durationMs: number;
	    machine: string;
	    os: string;
	    elevated: boolean;
	    alwaysAdmin: boolean;
	    totalRAMBytes: number;
	    totalRAMFormatted: string;
	
	    static createFrom(source: any = {}) {
	        return new Inventory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.apps = this.convertValues(source["apps"], AppGroup);
	        this.sources = this.convertValues(source["sources"], SourceInfo);
	        this.warnings = source["warnings"];
	        this.scannedAt = source["scannedAt"];
	        this.durationMs = source["durationMs"];
	        this.machine = source["machine"];
	        this.os = source["os"];
	        this.elevated = source["elevated"];
	        this.alwaysAdmin = source["alwaysAdmin"];
	        this.totalRAMBytes = source["totalRAMBytes"];
	        this.totalRAMFormatted = source["totalRAMFormatted"];
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
	export class PowerBlocker {
	    category: string;
	    caller: string;
	    reason: string;
	    exePath: string;
	
	    static createFrom(source: any = {}) {
	        return new PowerBlocker(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.category = source["category"];
	        this.caller = source["caller"];
	        this.reason = source["reason"];
	        this.exePath = source["exePath"];
	    }
	}
	export class PowerReport {
	    available: boolean;
	    requiresAdmin: boolean;
	    hasBlockers: boolean;
	    blockers: PowerBlocker[];
	    summary: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new PowerReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.requiresAdmin = source["requiresAdmin"];
	        this.hasBlockers = source["hasBlockers"];
	        this.blockers = this.convertValues(source["blockers"], PowerBlocker);
	        this.summary = source["summary"];
	        this.error = source["error"];
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
	export class ProcessPollResult {
	    totalRAMBytes: number;
	    totalRAMFormatted: string;
	    runningPIDs: number[];
	    activeExePaths: Record<string, Array<number>>;
	    ramByExePath: Record<string, number>;
	    ramFormattedByExe: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new ProcessPollResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalRAMBytes = source["totalRAMBytes"];
	        this.totalRAMFormatted = source["totalRAMFormatted"];
	        this.runningPIDs = source["runningPIDs"];
	        this.activeExePaths = source["activeExePaths"];
	        this.ramByExePath = source["ramByExePath"];
	        this.ramFormattedByExe = source["ramFormattedByExe"];
	    }
	}
	export class SentinelAlert {
	    id: string;
	    rootKey: string;
	    subKey: string;
	    name: string;
	    command: string;
	    timestamp: string;
	    action: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new SentinelAlert(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.rootKey = source["rootKey"];
	        this.subKey = source["subKey"];
	        this.name = source["name"];
	        this.command = source["command"];
	        this.timestamp = source["timestamp"];
	        this.action = source["action"];
	        this.message = source["message"];
	    }
	}
	export class SnapshotDiff {
	    baseId: string;
	    targetId: string;
	    baseName: string;
	    targetName: string;
	    added: Component[];
	    removed: Component[];
	    modified: ComponentDiff[];
	    summary: string;
	
	    static createFrom(source: any = {}) {
	        return new SnapshotDiff(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.baseId = source["baseId"];
	        this.targetId = source["targetId"];
	        this.baseName = source["baseName"];
	        this.targetName = source["targetName"];
	        this.added = this.convertValues(source["added"], Component);
	        this.removed = this.convertValues(source["removed"], Component);
	        this.modified = this.convertValues(source["modified"], ComponentDiff);
	        this.summary = source["summary"];
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
	export class SnapshotMeta {
	    id: string;
	    name: string;
	    createdAt: string;
	    itemCount: number;
	    appCount: number;
	
	    static createFrom(source: any = {}) {
	        return new SnapshotMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.createdAt = source["createdAt"];
	        this.itemCount = source["itemCount"];
	        this.appCount = source["appCount"];
	    }
	}
	

}

