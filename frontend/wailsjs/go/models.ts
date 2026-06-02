export namespace main {

	export class FileInfo {
	    name: string;
	    isDir: boolean;
	    size: number;
	    modTime: number;

	    static createFrom(source: any = {}) {
	        return new FileInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.isDir = source["isDir"];
	        this.size = source["size"];
	        this.modTime = source["modTime"];
	    }
	}

}

export namespace session {

	export class TerminalState {
	    type: string;
	    name: string;
	    minimized: boolean;
	    sshProfileId: string;
	    tmuxSessionName: string;

	    static createFrom(source: any = {}) {
	        return new TerminalState(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.name = source["name"];
	        this.minimized = source["minimized"];
	        this.sshProfileId = source["sshProfileId"];
	        this.tmuxSessionName = source["tmuxSessionName"] || '';
	    }
	}
	export class SessionData {
	    terminals: TerminalState[];

	    static createFrom(source: any = {}) {
	        return new SessionData(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.terminals = this.convertValues(source["terminals"], TerminalState);
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

export namespace ssh {

	export class Profile {
	    id: string;
	    name: string;
	    host: string;
	    port: number;
	    username: string;
	    authMethod: string;
	    keyPath: string;

	    static createFrom(source: any = {}) {
	        return new Profile(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.authMethod = source["authMethod"];
	        this.keyPath = source["keyPath"];
	    }
	}

	export class SSHKeyInfo {
	    name: string;
	    path: string;
	    type: string;

	    static createFrom(source: any = {}) {
	        return new SSHKeyInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.type = source["type"];
	    }
	}

}

export namespace template {

	export class Template {
	    name: string;
	    description: string;
	    commands: Record<string, string>;
	    favorite: boolean;

	    static createFrom(source: any = {}) {
	        return new Template(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.commands = source["commands"];
	        this.favorite = source["favorite"];
	    }
	}

}
