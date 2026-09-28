export namespace config {

	export class Application {
	    launchAtLogin: boolean;
	    minimizeToTray: boolean;

	    static createFrom(source: any = {}) {
	        return new Application(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.launchAtLogin = source["launchAtLogin"];
	        this.minimizeToTray = source["minimizeToTray"];
	    }
	}
	export class Server {
	    host: string;
	    port: number;
	    autoStart: boolean;
	    allowLan: boolean;
	    apiToken: string;
	
	    static createFrom(source: any = {}) {
	        return new Server(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.autoStart = source["autoStart"];
	        this.allowLan = source["allowLan"];
	        this.apiToken = source["apiToken"];
	    }
	}

}

export namespace logs {
	
	export class Entry {
	    id: number;
	    // Go type: time
	    time: any;
	    category: string;
	    level: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.time = this.convertValues(source["time"], null);
	        this.category = source["category"];
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

}

export namespace main {
	
	export class ConfigView {
	    server: config.Server;
	    application: config.Application;
	    obsHost: string;
	    obsPort: number;
	    autoConnect: boolean;
	    autoReconnect: boolean;
	    hasPassword: boolean;
	    activeProfile: string;
	    profileNames: string[];
	
	    static createFrom(source: any = {}) {
	        return new ConfigView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.server = this.convertValues(source["server"], config.Server);
	        this.application = this.convertValues(source["application"], config.Application);
	        this.obsHost = source["obsHost"];
	        this.obsPort = source["obsPort"];
	        this.autoConnect = source["autoConnect"];
	        this.autoReconnect = source["autoReconnect"];
	        this.hasPassword = source["hasPassword"];
	        this.activeProfile = source["activeProfile"];
	        this.profileNames = source["profileNames"];
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
	export class Snapshot {
	    serverRunning: boolean;
	    serverAddress: string;
	    obs: obs.Status;
	
	    static createFrom(source: any = {}) {
	        return new Snapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverRunning = source["serverRunning"];
	        this.serverAddress = source["serverAddress"];
	        this.obs = this.convertValues(source["obs"], obs.Status);
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

export namespace obs {
	
	export class Scene {
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new Scene(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	    }
	}
	export class Source {
	    sceneName: string;
	    name: string;
	    id: number;
	    enabled: boolean;

	    static createFrom(source: any = {}) {
	        return new Source(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sceneName = source["sceneName"];
	        this.name = source["name"];
	        this.id = source["id"];
	        this.enabled = source["enabled"];
	    }
	}
	export class Status {
	    connected: boolean;
	    currentScene: string;
	    recording: boolean;
	    streaming: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connected = source["connected"];
	        this.currentScene = source["currentScene"];
	        this.recording = source["recording"];
	        this.streaming = source["streaming"];
	    }
	}

}
