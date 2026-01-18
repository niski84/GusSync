export namespace services {
	
	export class CleanupRequest {
	    sourceRoot: string;
	    destRoot: string;
	    stateFiles: string[];
	    processBoth: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CleanupRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceRoot = source["sourceRoot"];
	        this.destRoot = source["destRoot"];
	        this.stateFiles = source["stateFiles"];
	        this.processBoth = source["processBoth"];
	    }
	}
	export class ConfigService {
	
	
	    static createFrom(source: any = {}) {
	        return new ConfigService(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}
	export class DeviceInfo {
	    id: string;
	    name: string;
	    type: string;
	    path: string;
	    connected: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DeviceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.path = source["path"];
	        this.connected = source["connected"];
	    }
	}
	export class JobInfo {
	    id: string;
	    type: string;
	    status: string;
	    // Go type: time
	    startTime: any;
	
	    static createFrom(source: any = {}) {
	        return new JobInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.status = source["status"];
	        this.startTime = this.convertValues(source["startTime"], null);
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
	export class LogEntry {
	    // Go type: time
	    timestamp: any;
	    level: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new LogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timestamp = this.convertValues(source["timestamp"], null);
	        this.level = source["level"];
	        this.message = source["message"];
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
	export class PrereqCheck {
	    id: string;
	    name: string;
	    status: string;
	    details: string;
	    remediationSteps: string[];
	    links?: string[];
	
	    static createFrom(source: any = {}) {
	        return new PrereqCheck(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.details = source["details"];
	        this.remediationSteps = source["remediationSteps"];
	        this.links = source["links"];
	    }
	}
	export class PrereqReport {
	    overallStatus: string;
	    os: string;
	    checks: PrereqCheck[];
	    // Go type: time
	    timestamp: any;
	
	    static createFrom(source: any = {}) {
	        return new PrereqReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.overallStatus = source["overallStatus"];
	        this.os = source["os"];
	        this.checks = this.convertValues(source["checks"], PrereqCheck);
	        this.timestamp = this.convertValues(source["timestamp"], null);
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
	export class StateFileInfo {
	    path: string;
	    mode: string;
	
	    static createFrom(source: any = {}) {
	        return new StateFileInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.mode = source["mode"];
	    }
	}
	export class VerifyRequest {
	    sourcePath: string;
	    destPath: string;
	    level: string;
	
	    static createFrom(source: any = {}) {
	        return new VerifyRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourcePath = source["sourcePath"];
	        this.destPath = source["destPath"];
	        this.level = source["level"];
	    }
	}

}

