package plugin

// AuditRecorder records plugin security events (RPC denials, start authorization).
type AuditRecorder func(pluginID, method string, denied bool, detail string)
