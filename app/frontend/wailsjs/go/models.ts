export namespace folder {
	
	export class Folder {
	    id: string;
	    name: string;
	    position: number;
	
	    static createFrom(source: any = {}) {
	        return new Folder(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.position = source["position"];
	    }
	}

}

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
	export class TerminalTypeOption {
	    value: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new TerminalTypeOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value = source["value"];
	        this.label = source["label"];
	    }
	}
	export class TranscriptListEntry {
	    resumeId: string;
	    name?: string;
	    type?: string;
	    sshProfileId?: string;
	    size: number;
	    // Go type: time
	    modTime: any;
	    active: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TranscriptListEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.resumeId = source["resumeId"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.sshProfileId = source["sshProfileId"];
	        this.size = source["size"];
	        this.modTime = this.convertValues(source["modTime"], null);
	        this.active = source["active"];
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

export namespace notes {
	
	export class NoteContent {
	    filename: string;
	    content: string;
	    modTime: number;
	
	    static createFrom(source: any = {}) {
	        return new NoteContent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.filename = source["filename"];
	        this.content = source["content"];
	        this.modTime = source["modTime"];
	    }
	}
	export class NoteInfo {
	    filename: string;
	    title: string;
	    modTime: number;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new NoteInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.filename = source["filename"];
	        this.title = source["title"];
	        this.modTime = source["modTime"];
	        this.size = source["size"];
	    }
	}

}

export namespace recording {
	
	export class AggDownloadInfo {
	    url: string;
	    destination: string;
	    platform: string;
	    version: string;
	    sha256: string;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new AggDownloadInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.destination = source["destination"];
	        this.platform = source["platform"];
	        this.version = source["version"];
	        this.sha256 = source["sha256"];
	        this.size = source["size"];
	    }
	}
	export class SessionMeta {
	    terminalId: number;
	    sessionType?: string;
	    sshHost?: string;
	    sshProfile?: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.terminalId = source["terminalId"];
	        this.sessionType = source["sessionType"];
	        this.sshHost = source["sshHost"];
	        this.sshProfile = source["sshProfile"];
	    }
	}
	export class RecordingInfo {
	    id: string;
	    title: string;
	    timestamp: number;
	    duration: number;
	    width: number;
	    height: number;
	    size: number;
	    meta?: SessionMeta;
	
	    static createFrom(source: any = {}) {
	        return new RecordingInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.timestamp = source["timestamp"];
	        this.duration = source["duration"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.size = source["size"];
	        this.meta = this.convertValues(source["meta"], SessionMeta);
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

export namespace session {
	
	export class TerminalState {
	    type: string;
	    name: string;
	    minimized: boolean;
	    sshProfileId?: string;
	    tmuxSessionName?: string;
	    resumeId?: string;
	    transcriptPath?: string;
	    restoreClass?: string;
	    folderId?: string;
	    background?: string;
	
	    static createFrom(source: any = {}) {
	        return new TerminalState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.name = source["name"];
	        this.minimized = source["minimized"];
	        this.sshProfileId = source["sshProfileId"];
	        this.tmuxSessionName = source["tmuxSessionName"];
	        this.resumeId = source["resumeId"];
	        this.transcriptPath = source["transcriptPath"];
	        this.restoreClass = source["restoreClass"];
	        this.folderId = source["folderId"];
	        this.background = source["background"];
	    }
	}
	export class SessionData {
	    terminals: TerminalState[];
	    layout?: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.terminals = this.convertValues(source["terminals"], TerminalState);
	        this.layout = source["layout"];
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
	
	export class FIDOChallenge {
	    credentialId: number[];
	    rpId: string;
	    challengeSalt: number[];
	    label?: string;
	
	    static createFrom(source: any = {}) {
	        return new FIDOChallenge(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.credentialId = source["credentialId"];
	        this.rpId = source["rpId"];
	        this.challengeSalt = source["challengeSalt"];
	        this.label = source["label"];
	    }
	}
	export class Profile {
	    id: string;
	    name: string;
	    host: string;
	    port: number;
	    username: string;
	    authMethod: string;
	    keyPath?: string;
	    jumpHostEnabled?: boolean;
	    jumpHost?: string;
	    jumpPort?: number;
	    jumpUsername?: string;
	    jumpAuthMethod?: string;
	    jumpKeyPath?: string;
	    useTmux?: boolean;
	    agentHost?: boolean;
	    rcMode?: string;
	    rcSnippet?: string;
	
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
	        this.jumpHostEnabled = source["jumpHostEnabled"];
	        this.jumpHost = source["jumpHost"];
	        this.jumpPort = source["jumpPort"];
	        this.jumpUsername = source["jumpUsername"];
	        this.jumpAuthMethod = source["jumpAuthMethod"];
	        this.jumpKeyPath = source["jumpKeyPath"];
	        this.useTmux = source["useTmux"];
	        this.agentHost = source["agentHost"];
	        this.rcMode = source["rcMode"];
	        this.rcSnippet = source["rcSnippet"];
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
	
	export class TemplateParameter {
	    name: string;
	    description?: string;
	    required: boolean;
	    type?: string;
	    pattern?: string;
	    maxLength?: number;
	    options?: string[];
	    source?: string;
	    discoveryTool?: string;
	
	    static createFrom(source: any = {}) {
	        return new TemplateParameter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.required = source["required"];
	        this.type = source["type"];
	        this.pattern = source["pattern"];
	        this.maxLength = source["maxLength"];
	        this.options = source["options"];
	        this.source = source["source"];
	        this.discoveryTool = source["discoveryTool"];
	    }
	}
	export class Template {
	    name: string;
	    description: string;
	    category?: string;
	    parameters?: TemplateParameter[];
	    toolEnabled: boolean;
	    dangerLevel?: string;
	    toolClass?: string;
	    commands: Record<string, string>;
	    favorite: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Template(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.category = source["category"];
	        this.parameters = this.convertValues(source["parameters"], TemplateParameter);
	        this.toolEnabled = source["toolEnabled"];
	        this.dangerLevel = source["dangerLevel"];
	        this.toolClass = source["toolClass"];
	        this.commands = source["commands"];
	        this.favorite = source["favorite"];
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

export namespace transcript {
	
	export class Content {
	    resumeId: string;
	    text: string;
	    size: number;
	    readBytes: number;
	    truncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Content(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.resumeId = source["resumeId"];
	        this.text = source["text"];
	        this.size = source["size"];
	        this.readBytes = source["readBytes"];
	        this.truncated = source["truncated"];
	    }
	}
	export class DeleteResult {
	    resumeId: string;
	    ok: boolean;
	    reason?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new DeleteResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.resumeId = source["resumeId"];
	        this.ok = source["ok"];
	        this.reason = source["reason"];
	        this.error = source["error"];
	    }
	}
	export class DiskUsageInfo {
	    count: number;
	    totalBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new DiskUsageInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.count = source["count"];
	        this.totalBytes = source["totalBytes"];
	    }
	}

}

