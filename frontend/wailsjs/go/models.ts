export namespace main {
	
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
	    lastRun: string;
	    publisher: string;
	    product: string;
	    signature: string;
	    locked: string;
	    system: boolean;
	    review: boolean;
	    flags: string[];
	
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
	        this.lastRun = source["lastRun"];
	        this.publisher = source["publisher"];
	        this.product = source["product"];
	        this.signature = source["signature"];
	        this.locked = source["locked"];
	        this.system = source["system"];
	        this.review = source["review"];
	        this.flags = source["flags"];
	    }
	}
	export class AppGroup {
	    id: string;
	    name: string;
	    publisher: string;
	    letter: string;
	    signature: string;
	    path: string;
	    components: Component[];
	    running: boolean;
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
	        this.letter = source["letter"];
	        this.signature = source["signature"];
	        this.path = source["path"];
	        this.components = this.convertValues(source["components"], Component);
	        this.running = source["running"];
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

