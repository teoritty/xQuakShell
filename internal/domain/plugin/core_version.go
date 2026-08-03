package plugin

// HostCoreVersion is the core (backend engine) version. It is purely informational —
// reported to plugins at initialize and rendered in the About panel. Plugins gate on the
// pluginApi envelope + capability versions instead (ADR-012), never on this value.
const HostCoreVersion = "1.0.0"
