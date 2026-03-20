export namespace usecase {
	
	export class PingResult {
	    connectionId: string;
	    reachable: boolean;
	    latencyMs: number;
	
	    static createFrom(source: any = {}) {
	        return new PingResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connectionId = source["connectionId"];
	        this.reachable = source["reachable"];
	        this.latencyMs = source["latencyMs"];
	    }
	}

}

export namespace wails {
	
	export class AppSettingsDTO {
	    lockoutEnabled: boolean;
	    lockoutIdleMinutes: number;
	    lockOnMinimize: boolean;
	    terminalFontFamily: string;
	    terminalFontSize: number;
	    terminalFontColor: string;
	    theme: string;
	    pingEnabled: boolean;
	    pingMode: string;
	    pingIntervalSeconds: number;
	    pingIntervalMin: number;
	    externalEditorPath: string;
	    transferSpeedLimitKbps: number;
	    connectionTimeoutSeconds: number;
	    maxConcurrentTransfers: number;
	    sessionHotkeyCreate: string;
	    sessionHotkeyNext: string;
	    sessionHotkeyPrev: string;
	    sessionHotkeyClose: string;
	
	    static createFrom(source: any = {}) {
	        return new AppSettingsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lockoutEnabled = source["lockoutEnabled"];
	        this.lockoutIdleMinutes = source["lockoutIdleMinutes"];
	        this.lockOnMinimize = source["lockOnMinimize"];
	        this.terminalFontFamily = source["terminalFontFamily"];
	        this.terminalFontSize = source["terminalFontSize"];
	        this.terminalFontColor = source["terminalFontColor"];
	        this.theme = source["theme"];
	        this.pingEnabled = source["pingEnabled"];
	        this.pingMode = source["pingMode"];
	        this.pingIntervalSeconds = source["pingIntervalSeconds"];
	        this.pingIntervalMin = source["pingIntervalMin"];
	        this.externalEditorPath = source["externalEditorPath"];
	        this.transferSpeedLimitKbps = source["transferSpeedLimitKbps"];
	        this.connectionTimeoutSeconds = source["connectionTimeoutSeconds"];
	        this.maxConcurrentTransfers = source["maxConcurrentTransfers"];
	        this.sessionHotkeyCreate = source["sessionHotkeyCreate"];
	        this.sessionHotkeyNext = source["sessionHotkeyNext"];
	        this.sessionHotkeyPrev = source["sessionHotkeyPrev"];
	        this.sessionHotkeyClose = source["sessionHotkeyClose"];
	    }
	}
	export class AuditEntryDTO {
	    id: number;
	    timestamp: string;
	    sessionId: string;
	    connectionId: string;
	    username: string;
	    input: string;
	    redacted: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AuditEntryDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.timestamp = source["timestamp"];
	        this.sessionId = source["sessionId"];
	        this.connectionId = source["connectionId"];
	        this.username = source["username"];
	        this.input = source["input"];
	        this.redacted = source["redacted"];
	    }
	}
	export class HTTPConfigDTO {
	    url: string;
	    method: string;
	    auth?: string;
	    passwordId?: string;
	
	    static createFrom(source: any = {}) {
	        return new HTTPConfigDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.method = source["method"];
	        this.auth = source["auth"];
	        this.passwordId = source["passwordId"];
	    }
	}
	export class SerialConfigDTO {
	    port: string;
	    baudRate: number;
	    dataBits: number;
	    stopBits: number;
	    parity: string;
	
	    static createFrom(source: any = {}) {
	        return new SerialConfigDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.port = source["port"];
	        this.baudRate = source["baudRate"];
	        this.dataBits = source["dataBits"];
	        this.stopBits = source["stopBits"];
	        this.parity = source["parity"];
	    }
	}
	export class RDPConfigDTO {
	    host: string;
	    port: number;
	    username?: string;
	    passwordId?: string;
	    domain?: string;
	
	    static createFrom(source: any = {}) {
	        return new RDPConfigDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.passwordId = source["passwordId"];
	        this.domain = source["domain"];
	    }
	}
	export class TelnetConfigDTO {
	    host: string;
	    port: number;
	    username?: string;
	    passwordId?: string;
	
	    static createFrom(source: any = {}) {
	        return new TelnetConfigDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.passwordId = source["passwordId"];
	    }
	}
	export class ProxyDTO {
	    type: string;
	    host: string;
	    port: number;
	    username?: string;
	    passwordId?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProxyDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.passwordId = source["passwordId"];
	    }
	}
	export class JumpHopDTO {
	    host: string;
	    port: number;
	    username: string;
	    authMethod: string;
	    keyAuth?: KeyAuthConfigDTO;
	    passAuth?: PassAuthConfigDTO;
	
	    static createFrom(source: any = {}) {
	        return new JumpHopDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.authMethod = source["authMethod"];
	        this.keyAuth = this.convertValues(source["keyAuth"], KeyAuthConfigDTO);
	        this.passAuth = this.convertValues(source["passAuth"], PassAuthConfigDTO);
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
	export class PassAuthConfigDTO {
	    passwordId: string;
	
	    static createFrom(source: any = {}) {
	        return new PassAuthConfigDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.passwordId = source["passwordId"];
	    }
	}
	export class KeyAuthConfigDTO {
	    identityIds: string[];
	
	    static createFrom(source: any = {}) {
	        return new KeyAuthConfigDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.identityIds = source["identityIds"];
	    }
	}
	export class ConnectionUserDTO {
	    id: string;
	    username: string;
	    authMethod: string;
	    keyAuth?: KeyAuthConfigDTO;
	    passAuth?: PassAuthConfigDTO;
	    label?: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionUserDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.username = source["username"];
	        this.authMethod = source["authMethod"];
	        this.keyAuth = this.convertValues(source["keyAuth"], KeyAuthConfigDTO);
	        this.passAuth = this.convertValues(source["passAuth"], PassAuthConfigDTO);
	        this.label = source["label"];
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
	export class ConnectionDTO {
	    id: string;
	    folderId: string;
	    name: string;
	    host: string;
	    port: number;
	    order: number;
	    protocol?: string;
	    user?: string;
	    identityIds?: string[];
	    users?: ConnectionUserDTO[];
	    defaultUserId?: string;
	    tags?: string[];
	    vpnProfileId?: string;
	    jumpChain?: JumpHopDTO[];
	    proxy?: ProxyDTO;
	    telnetConfig?: TelnetConfigDTO;
	    rdpConfig?: RDPConfigDTO;
	    serialConfig?: SerialConfigDTO;
	    httpConfig?: HTTPConfigDTO;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.folderId = source["folderId"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.order = source["order"];
	        this.protocol = source["protocol"];
	        this.user = source["user"];
	        this.identityIds = source["identityIds"];
	        this.users = this.convertValues(source["users"], ConnectionUserDTO);
	        this.defaultUserId = source["defaultUserId"];
	        this.tags = source["tags"];
	        this.vpnProfileId = source["vpnProfileId"];
	        this.jumpChain = this.convertValues(source["jumpChain"], JumpHopDTO);
	        this.proxy = this.convertValues(source["proxy"], ProxyDTO);
	        this.telnetConfig = this.convertValues(source["telnetConfig"], TelnetConfigDTO);
	        this.rdpConfig = this.convertValues(source["rdpConfig"], RDPConfigDTO);
	        this.serialConfig = this.convertValues(source["serialConfig"], SerialConfigDTO);
	        this.httpConfig = this.convertValues(source["httpConfig"], HTTPConfigDTO);
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
	
	export class FolderDTO {
	    id: string;
	    name: string;
	    parentId: string;
	    order: number;
	
	    static createFrom(source: any = {}) {
	        return new FolderDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.parentId = source["parentId"];
	        this.order = source["order"];
	    }
	}
	
	export class IdentityDTO {
	    id: string;
	    comment: string;
	    keyType: string;
	    encrypted: boolean;
	
	    static createFrom(source: any = {}) {
	        return new IdentityDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.comment = source["comment"];
	        this.keyType = source["keyType"];
	        this.encrypted = source["encrypted"];
	    }
	}
	
	
	export class KnownHostDTO {
	    host: string;
	    keyType: string;
	    fingerprint: string;
	
	    static createFrom(source: any = {}) {
	        return new KnownHostDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.keyType = source["keyType"];
	        this.fingerprint = source["fingerprint"];
	    }
	}
	export class LocalNodeDTO {
	    name: string;
	    path: string;
	    isDir: boolean;
	    size: number;
	    modTime?: string;
	    mode?: string;
	    owner?: string;
	
	    static createFrom(source: any = {}) {
	        return new LocalNodeDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.isDir = source["isDir"];
	        this.size = source["size"];
	        this.modTime = source["modTime"];
	        this.mode = source["mode"];
	        this.owner = source["owner"];
	    }
	}
	
	
	export class PuTTYSessionDTO {
	    name: string;
	    hostName: string;
	    port: number;
	    userName: string;
	
	    static createFrom(source: any = {}) {
	        return new PuTTYSessionDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.hostName = source["hostName"];
	        this.port = source["port"];
	        this.userName = source["userName"];
	    }
	}
	
	export class RemoteNodeDTO {
	    path: string;
	    name: string;
	    isDir: boolean;
	    size: number;
	    modTime: string;
	    mode?: string;
	    owner?: string;
	    group?: string;
	
	    static createFrom(source: any = {}) {
	        return new RemoteNodeDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.isDir = source["isDir"];
	        this.size = source["size"];
	        this.modTime = source["modTime"];
	        this.mode = source["mode"];
	        this.owner = source["owner"];
	        this.group = source["group"];
	    }
	}
	
	export class SessionDTO {
	    sessionId: string;
	    connectionId: string;
	    connectionName: string;
	    protocol?: string;
	    state: string;
	    errorMessage: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.connectionId = source["connectionId"];
	        this.connectionName = source["connectionName"];
	        this.protocol = source["protocol"];
	        this.state = source["state"];
	        this.errorMessage = source["errorMessage"];
	    }
	}
	
	export class VPNProfileDTO {
	    id: string;
	    label: string;
	    protocol: string;
	
	    static createFrom(source: any = {}) {
	        return new VPNProfileDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.protocol = source["protocol"];
	    }
	}

}

