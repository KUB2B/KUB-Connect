export namespace app {
	
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
	
	    static createFrom(source: any = {}) {
	        return new SettingsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.autoConnect = source["autoConnect"];
	        this.autoStart = source["autoStart"];
	        this.killSwitch = source["killSwitch"];
	    }
	}
	export class StateDTO {
	    servers: ServerDTO[];
	    activeServer: number;
	    profile: ProfileDTO;
	    settings: SettingsDTO;
	    conn: string;
	    lastError: string;
	
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

