// Package appinfo holds build-level product metadata that is not tied to any domain concept.
package appinfo

// AppVersion is the user-facing application/product version, shown in the About panel. It is the
// release version of xQuakShell as a whole, distinct from the plugin core version and the frozen
// plugin API envelope version (ADR-012). Keep it in sync with wails.json productVersion.
const AppVersion = "1.0.0"
