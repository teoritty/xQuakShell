export namespace wails {
	
	export class AppSettingsDTO {
	    lockoutEnabled: boolean;
	    lockoutIdleMinutes: number;
	    lockOnMinimize: boolean;
	    terminalFontFamily: string;
	    terminalFontSize: number;
	    terminalFontColor: string;
	    theme: string;
	    uiScalePercent: number;
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
	    auditLogEnabled: boolean;
	    auditRetentionMode: string;
	    auditRetentionDays: number;
	    auditRetentionCount: number;
	    auditShowUsername: boolean;
	    auditShowConnection: boolean;
	
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
	        this.uiScalePercent = source["uiScalePercent"];
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
	        this.auditLogEnabled = source["auditLogEnabled"];
	        this.auditRetentionMode = source["auditRetentionMode"];
	        this.auditRetentionDays = source["auditRetentionDays"];
	        this.auditRetentionCount = source["auditRetentionCount"];
	        this.auditShowUsername = source["auditShowUsername"];
	        this.auditShowConnection = source["auditShowConnection"];
	    }
	}
	export class AuditEntryDTO {
	    id: number;
	    timestamp: string;
	    sessionId: string;
	    connectionId: string;
	    connectionName: string;
	    host: string;
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
	        this.connectionName = source["connectionName"];
	        this.host = source["host"];
	        this.username = source["username"];
	        this.input = source["input"];
	        this.redacted = source["redacted"];
	    }
	}
	export class AuditSessionStateDTO {
	    logSecretsEnabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AuditSessionStateDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.logSecretsEnabled = source["logSecretsEnabled"];
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
	    jumpChain?: JumpHopDTO[];
	    proxy?: ProxyDTO;
	
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
	        this.jumpChain = this.convertValues(source["jumpChain"], JumpHopDTO);
	        this.proxy = this.convertValues(source["proxy"], ProxyDTO);
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
	export class ConnectionProtocolDTO {
	    id: string;
	    label: string;
	    defaultPort?: number;
	    icon?: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionProtocolDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.defaultPort = source["defaultPort"];
	        this.icon = source["icon"];
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
	
	export class PingResultDTO {
	    connectionId: string;
	    reachable: boolean;
	    latencyMs: number;
	
	    static createFrom(source: any = {}) {
	        return new PingResultDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connectionId = source["connectionId"];
	        this.reachable = source["reachable"];
	        this.latencyMs = source["latencyMs"];
	    }
	}
	export class PluginCommandDTO {
	    pluginId: string;
	    id: string;
	    fullId: string;
	    title: string;
	    category?: string;
	
	    static createFrom(source: any = {}) {
	        return new PluginCommandDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pluginId = source["pluginId"];
	        this.id = source["id"];
	        this.fullId = source["fullId"];
	        this.title = source["title"];
	        this.category = source["category"];
	    }
	}
	export class PluginStatusBarDTO {
	    pluginId: string;
	    id: string;
	    text: string;
	    tooltip?: string;
	    priority?: number;
	
	    static createFrom(source: any = {}) {
	        return new PluginStatusBarDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pluginId = source["pluginId"];
	        this.id = source["id"];
	        this.text = source["text"];
	        this.tooltip = source["tooltip"];
	        this.priority = source["priority"];
	    }
	}
	export class PluginViewDTO {
	    pluginId: string;
	    id: string;
	    fullId: string;
	    location: string;
	    title: string;
	    type?: string;
	    entry?: string;
	    assetUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new PluginViewDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pluginId = source["pluginId"];
	        this.id = source["id"];
	        this.fullId = source["fullId"];
	        this.location = source["location"];
	        this.title = source["title"];
	        this.type = source["type"];
	        this.entry = source["entry"];
	        this.assetUrl = source["assetUrl"];
	    }
	}
	export class PluginContributionsDTO {
	    commands: PluginCommandDTO[];
	    views: PluginViewDTO[];
	    statusBar: PluginStatusBarDTO[];
	
	    static createFrom(source: any = {}) {
	        return new PluginContributionsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.commands = this.convertValues(source["commands"], PluginCommandDTO);
	        this.views = this.convertValues(source["views"], PluginViewDTO);
	        this.statusBar = this.convertValues(source["statusBar"], PluginStatusBarDTO);
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
	export class PluginDTO {
	    id: string;
	    name: string;
	    version: string;
	    description: string;
	    source: string;
	    state: string;
	    requiresSecretAccess: boolean;
	    signed: boolean;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PluginDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.version = source["version"];
	        this.description = source["description"];
	        this.source = source["source"];
	        this.state = source["state"];
	        this.requiresSecretAccess = source["requiresSecretAccess"];
	        this.signed = source["signed"];
	        this.enabled = source["enabled"];
	    }
	}
	export class PluginInstallPreviewDTO {
	    id: string;
	    name: string;
	    version: string;
	    description: string;
	    signed: boolean;
	    signatureVerified: boolean;
	    checksumPresent: boolean;
	    requiresSecretAccess: boolean;
	    multiSessionWarning: boolean;
	    unsignedWarning: boolean;
	    untrustedSignatureWarning: boolean;
	    permissions: string[];
	
	    static createFrom(source: any = {}) {
	        return new PluginInstallPreviewDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.version = source["version"];
	        this.description = source["description"];
	        this.signed = source["signed"];
	        this.signatureVerified = source["signatureVerified"];
	        this.checksumPresent = source["checksumPresent"];
	        this.requiresSecretAccess = source["requiresSecretAccess"];
	        this.multiSessionWarning = source["multiSessionWarning"];
	        this.unsignedWarning = source["unsignedWarning"];
	        this.untrustedSignatureWarning = source["untrustedSignatureWarning"];
	        this.permissions = source["permissions"];
	    }
	}
	export class PluginPingResultDTO {
	    pluginId: string;
	    result: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new PluginPingResultDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pluginId = source["pluginId"];
	        this.result = source["result"];
	    }
	}
	export class PluginPublisherKeyPairDTO {
	    publicKey: string;
	    privateKey: string;
	
	    static createFrom(source: any = {}) {
	        return new PluginPublisherKeyPairDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.publicKey = source["publicKey"];
	        this.privateKey = source["privateKey"];
	    }
	}
	export class PluginSettingsDTO {
	    trustedPublisherKeys: string[];
	    requireSignedPlugins: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PluginSettingsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.trustedPublisherKeys = source["trustedPublisherKeys"];
	        this.requireSignedPlugins = source["requireSignedPlugins"];
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

}

