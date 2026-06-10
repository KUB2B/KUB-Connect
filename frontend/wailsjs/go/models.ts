export namespace app {
	
	export class CapsDTO {
	    os: string;
	    version: string;
	    tunSupported: boolean;
	    killSwitchSupported: boolean;
	    elevated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CapsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.os = source["os"];
	        this.version = source["version"];
	        this.tunSupported = source["tunSupported"];
	        this.killSwitchSupported = source["killSwitchSupported"];
	        this.elevated = source["elevated"];
	    }
	}
	export class ProfileDTO {
	    telegram: boolean;
	    forceRUDirect: boolean;
	    customProxyDomains: string[];
	    customProxyIPs: string[];
	
	    static createFrom(source: any = {}) {
	        return new ProfileDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.telegram = source["telegram"];
	        this.forceRUDirect = source["forceRUDirect"];
	        this.customProxyDomains = source["customProxyDomains"];
	        this.customProxyIPs = source["customProxyIPs"];
	    }
	}
	export class ServerDTO {
	    name: string;
	    host: string;
	    port: number;
	    security: string;
	    network: string;
	
	    static createFrom(source: any = {}) {
	        return new ServerDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.security = source["security"];
	        this.network = source["network"];
	    }
	}
	export class SettingsDTO {
	    mode: string;
	    autoConnect: boolean;
	    autoStart: boolean;
	    killSwitch: boolean;
	    mux: boolean;
	    logLevel: string;
	
	    static createFrom(source: any = {}) {
	        return new SettingsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.autoConnect = source["autoConnect"];
	        this.autoStart = source["autoStart"];
	        this.killSwitch = source["killSwitch"];
	        this.mux = source["mux"];
	        this.logLevel = source["logLevel"];
	    }
	}
	export class StateDTO {
	    servers: ServerDTO[];
	    activeServer: number;
	    profile: ProfileDTO;
	    settings: SettingsDTO;
	    conn: string;
	    lastError: string;
	    caps: CapsDTO;
	
	    static createFrom(source: any = {}) {
	        return new StateDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.servers = this.convertValues(source["servers"], ServerDTO);
	        this.activeServer = source["activeServer"];
	        this.profile = this.convertValues(source["profile"], ProfileDTO);
	        this.settings = this.convertValues(source["settings"], SettingsDTO);
	        this.conn = source["conn"];
	        this.lastError = source["lastError"];
	        this.caps = this.convertValues(source["caps"], CapsDTO);
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

export namespace main {
	
	export class UpdateInfo {
	    available: boolean;
	    version: string;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.version = source["version"];
	        this.url = source["url"];
	    }
	}

}

