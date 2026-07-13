export namespace wails {
	
	export class AddGitHubRepositoryRequest {
	    url: string;
	    trusted: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AddGitHubRepositoryRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.trusted = source["trusted"];
	    }
	}
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
	    maxConcurrentPings: number;
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
	    debugLogWindowEnabled: boolean;
	
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
	        this.maxConcurrentPings = source["maxConcurrentPings"];
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
	        this.debugLogWindowEnabled = source["debugLogWindowEnabled"];
	    }
	}
	export class AuditEntryDTO {
	    id: number;
	    timestamp: string;
	    category: string;
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
	        this.category = source["category"];
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
	export class ForwardRuleDTO {
	    id: string;
	    kind: string;
	    bindAddress: string;
	    bindPort: number;
	    targetHost?: string;
	    targetPort?: number;
	    pluginId?: string;
	    providerId?: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ForwardRuleDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.bindAddress = source["bindAddress"];
	        this.bindPort = source["bindPort"];
	        this.targetHost = source["targetHost"];
	        this.targetPort = source["targetPort"];
	        this.pluginId = source["pluginId"];
	        this.providerId = source["providerId"];
	        this.enabled = source["enabled"];
	    }
	}
	export class JumpHopDTO {
	    id: string;
	    host: string;
	    port: number;
	    username: string;
	    authMethod: string;
	    keyAuth?: KeyAuthConfigDTO;
	    passAuth?: PassAuthConfigDTO;
	    pluginAuth?: PluginAuthConfigDTO;
	
	    static createFrom(source: any = {}) {
	        return new JumpHopDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.authMethod = source["authMethod"];
	        this.keyAuth = this.convertValues(source["keyAuth"], KeyAuthConfigDTO);
	        this.passAuth = this.convertValues(source["passAuth"], PassAuthConfigDTO);
	        this.pluginAuth = this.convertValues(source["pluginAuth"], PluginAuthConfigDTO);
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
	export class PluginAuthConfigDTO {
	    pluginId: string;
	    authMethodId: string;
	    fields?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new PluginAuthConfigDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pluginId = source["pluginId"];
	        this.authMethodId = source["authMethodId"];
	        this.fields = source["fields"];
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
	    pluginAuth?: PluginAuthConfigDTO;
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
	        this.pluginAuth = this.convertValues(source["pluginAuth"], PluginAuthConfigDTO);
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
	    users?: ConnectionUserDTO[];
	    defaultUserId?: string;
	    tags?: string[];
	    jumpChain?: JumpHopDTO[];
	    pluginFields?: Record<string, string>;
	    forwardRules?: ForwardRuleDTO[];
	
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
	        this.users = this.convertValues(source["users"], ConnectionUserDTO);
	        this.defaultUserId = source["defaultUserId"];
	        this.tags = source["tags"];
	        this.jumpChain = this.convertValues(source["jumpChain"], JumpHopDTO);
	        this.pluginFields = source["pluginFields"];
	        this.forwardRules = this.convertValues(source["forwardRules"], ForwardRuleDTO);
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
	export class FieldOptionDTO {
	    value: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new FieldOptionDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value = source["value"];
	        this.label = source["label"];
	    }
	}
	export class FieldValidationDTO {
	    minLength?: number;
	    maxLength?: number;
	    min?: number;
	    max?: number;
	    pattern?: string;
	    maxSizeBytes?: number;
	
	    static createFrom(source: any = {}) {
	        return new FieldValidationDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.minLength = source["minLength"];
	        this.maxLength = source["maxLength"];
	        this.min = source["min"];
	        this.max = source["max"];
	        this.pattern = source["pattern"];
	        this.maxSizeBytes = source["maxSizeBytes"];
	    }
	}
	export class FieldDefDTO {
	    id: string;
	    label: string;
	    type: string;
	    required: boolean;
	    default?: any;
	    placeholder?: string;
	    description?: string;
	    width?: string;
	    order: number;
	    validation?: FieldValidationDTO;
	    options?: FieldOptionDTO[];
	    dependsOn?: string;
	    secret: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FieldDefDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.type = source["type"];
	        this.required = source["required"];
	        this.default = source["default"];
	        this.placeholder = source["placeholder"];
	        this.description = source["description"];
	        this.width = source["width"];
	        this.order = source["order"];
	        this.validation = this.convertValues(source["validation"], FieldValidationDTO);
	        this.options = this.convertValues(source["options"], FieldOptionDTO);
	        this.dependsOn = source["dependsOn"];
	        this.secret = source["secret"];
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
	export class FieldGroupDTO {
	    id: string;
	    label: string;
	    order: number;
	    fields: FieldDefDTO[];
	
	    static createFrom(source: any = {}) {
	        return new FieldGroupDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.order = source["order"];
	        this.fields = this.convertValues(source["fields"], FieldDefDTO);
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
	    surface?: string;
	    remoteFs?: boolean;
	    fields?: FieldGroupDTO[];
	
	    static createFrom(source: any = {}) {
	        return new ConnectionProtocolDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.defaultPort = source["defaultPort"];
	        this.icon = source["icon"];
	        this.surface = source["surface"];
	        this.remoteFs = source["remoteFs"];
	        this.fields = this.convertValues(source["fields"], FieldGroupDTO);
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
	
	export class FetchGitHubPluginsRequest {
	    url: string;
	    forceRefresh: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FetchGitHubPluginsRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.forceRefresh = source["forceRefresh"];
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
	
	export class GitHubReleaseSummaryDTO {
	    tag: string;
	    name: string;
	    publishedAt: string;
	    prerelease: boolean;
	    platformSupported: boolean;
	    platforms: PlatformInfoDTO[];
	
	    static createFrom(source: any = {}) {
	        return new GitHubReleaseSummaryDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tag = source["tag"];
	        this.name = source["name"];
	        this.publishedAt = source["publishedAt"];
	        this.prerelease = source["prerelease"];
	        this.platformSupported = source["platformSupported"];
	        this.platforms = this.convertValues(source["platforms"], PlatformInfoDTO);
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
	export class PlatformInfoDTO {
	    os: string;
	    arch: string;
	    assetName: string;
	
	    static createFrom(source: any = {}) {
	        return new PlatformInfoDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.os = source["os"];
	        this.arch = source["arch"];
	        this.assetName = source["assetName"];
	    }
	}
	export class GitHubPluginMetadataDTO {
	    repositoryUrl: string;
	    id: string;
	    name: string;
	    version: string;
	    description: string;
	    author: string;
	    license: string;
	    platforms: PlatformInfoDTO[];
	    availableReleases: GitHubReleaseSummaryDTO[];
	    latestRelease: string;
	    prerelease: boolean;
	    publishedAt: string;
	    readme: string;
	    minCoreVersion: string;
	    platformSupported: boolean;
	    installed: boolean;
	    installedVersion: string;
	    installedReleaseTag: string;
	
	    static createFrom(source: any = {}) {
	        return new GitHubPluginMetadataDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repositoryUrl = source["repositoryUrl"];
	        this.id = source["id"];
	        this.name = source["name"];
	        this.version = source["version"];
	        this.description = source["description"];
	        this.author = source["author"];
	        this.license = source["license"];
	        this.platforms = this.convertValues(source["platforms"], PlatformInfoDTO);
	        this.availableReleases = this.convertValues(source["availableReleases"], GitHubReleaseSummaryDTO);
	        this.latestRelease = source["latestRelease"];
	        this.prerelease = source["prerelease"];
	        this.publishedAt = source["publishedAt"];
	        this.readme = source["readme"];
	        this.minCoreVersion = source["minCoreVersion"];
	        this.platformSupported = source["platformSupported"];
	        this.installed = source["installed"];
	        this.installedVersion = source["installedVersion"];
	        this.installedReleaseTag = source["installedReleaseTag"];
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
	export class GitHubPluginListDTO {
	    repositoryUrl: string;
	    plugins: GitHubPluginMetadataDTO[];
	
	    static createFrom(source: any = {}) {
	        return new GitHubPluginListDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repositoryUrl = source["repositoryUrl"];
	        this.plugins = this.convertValues(source["plugins"], GitHubPluginMetadataDTO);
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
	
	export class GitHubPluginPreviewResponseDTO {
	    repositoryUrl: string;
	    repositoryTrusted: boolean;
	    id: string;
	    name: string;
	    version: string;
	    description: string;
	    author: string;
	    license: string;
	    minCoreVersion: string;
	    currentPlatform: string;
	    platformSupported: boolean;
	    supportedPlatforms: string[];
	    latestRelease: string;
	    releaseTag: string;
	    prerelease: boolean;
	    publishedDate: string;
	    readme: string;
	    requiresSecretAccess: boolean;
	    requiresAuthProviderAccess: boolean;
	    requiresTunnelProviderAccess: boolean;
	    multiSessionWarning: boolean;
	    arbitraryNetworkWarning: boolean;
	    unsignedPlugin: boolean;
	    untrustedSource: boolean;
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new GitHubPluginPreviewResponseDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repositoryUrl = source["repositoryUrl"];
	        this.repositoryTrusted = source["repositoryTrusted"];
	        this.id = source["id"];
	        this.name = source["name"];
	        this.version = source["version"];
	        this.description = source["description"];
	        this.author = source["author"];
	        this.license = source["license"];
	        this.minCoreVersion = source["minCoreVersion"];
	        this.currentPlatform = source["currentPlatform"];
	        this.platformSupported = source["platformSupported"];
	        this.supportedPlatforms = source["supportedPlatforms"];
	        this.latestRelease = source["latestRelease"];
	        this.releaseTag = source["releaseTag"];
	        this.prerelease = source["prerelease"];
	        this.publishedDate = source["publishedDate"];
	        this.readme = source["readme"];
	        this.requiresSecretAccess = source["requiresSecretAccess"];
	        this.requiresAuthProviderAccess = source["requiresAuthProviderAccess"];
	        this.requiresTunnelProviderAccess = source["requiresTunnelProviderAccess"];
	        this.multiSessionWarning = source["multiSessionWarning"];
	        this.arbitraryNetworkWarning = source["arbitraryNetworkWarning"];
	        this.unsignedPlugin = source["unsignedPlugin"];
	        this.untrustedSource = source["untrustedSource"];
	        this.warnings = source["warnings"];
	    }
	}
	
	export class GitHubRepositoryDTO {
	    url: string;
	    owner: string;
	    repo: string;
	    displayName: string;
	    addedAt: string;
	    lastFetchedAt?: string;
	    trusted: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GitHubRepositoryDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.owner = source["owner"];
	        this.repo = source["repo"];
	        this.displayName = source["displayName"];
	        this.addedAt = source["addedAt"];
	        this.lastFetchedAt = source["lastFetchedAt"];
	        this.trusted = source["trusted"];
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
	
	
	export class PluginAuthMethodDTO {
	    pluginId: string;
	    id: string;
	    label: string;
	    kind: string;
	    fields?: FieldGroupDTO[];
	
	    static createFrom(source: any = {}) {
	        return new PluginAuthMethodDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pluginId = source["pluginId"];
	        this.id = source["id"];
	        this.label = source["label"];
	        this.kind = source["kind"];
	        this.fields = this.convertValues(source["fields"], FieldGroupDTO);
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
	export class PluginTunnelProviderDTO {
	    pluginId: string;
	    id: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new PluginTunnelProviderDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pluginId = source["pluginId"];
	        this.id = source["id"];
	        this.label = source["label"];
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
	    authMethods: PluginAuthMethodDTO[];
	    tunnelProviders: PluginTunnelProviderDTO[];
	
	    static createFrom(source: any = {}) {
	        return new PluginContributionsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.commands = this.convertValues(source["commands"], PluginCommandDTO);
	        this.views = this.convertValues(source["views"], PluginViewDTO);
	        this.statusBar = this.convertValues(source["statusBar"], PluginStatusBarDTO);
	        this.authMethods = this.convertValues(source["authMethods"], PluginAuthMethodDTO);
	        this.tunnelProviders = this.convertValues(source["tunnelProviders"], PluginTunnelProviderDTO);
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
	    requiresAuthProviderAccess: boolean;
	    requiresTunnelProviderAccess: boolean;
	    multiSessionWarning: boolean;
	    arbitraryNetworkWarning: boolean;
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
	        this.requiresAuthProviderAccess = source["requiresAuthProviderAccess"];
	        this.requiresTunnelProviderAccess = source["requiresTunnelProviderAccess"];
	        this.multiSessionWarning = source["multiSessionWarning"];
	        this.arbitraryNetworkWarning = source["arbitraryNetworkWarning"];
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
	export class SessionEmbedDTO {
	    uiUrl: string;
	    tunnelUrl: string;
	    sandbox: string[];
	
	    static createFrom(source: any = {}) {
	        return new SessionEmbedDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uiUrl = source["uiUrl"];
	        this.tunnelUrl = source["tunnelUrl"];
	        this.sandbox = source["sandbox"];
	    }
	}
	export class SessionDTO {
	    sessionId: string;
	    connectionId: string;
	    connectionName: string;
	    protocol?: string;
	    surface?: string;
	    state: string;
	    errorMessage: string;
	    embed?: SessionEmbedDTO;
	
	    static createFrom(source: any = {}) {
	        return new SessionDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.connectionId = source["connectionId"];
	        this.connectionName = source["connectionName"];
	        this.protocol = source["protocol"];
	        this.surface = source["surface"];
	        this.state = source["state"];
	        this.errorMessage = source["errorMessage"];
	        this.embed = this.convertValues(source["embed"], SessionEmbedDTO);
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
	
	export class SetGitHubRepositoryTrustRequest {
	    url: string;
	    trusted: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SetGitHubRepositoryTrustRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.trusted = source["trusted"];
	    }
	}

}

