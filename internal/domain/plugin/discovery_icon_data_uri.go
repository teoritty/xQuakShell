package plugin

import (
	"encoding/base64"
	"path"
	"strings"
)

// discoveryIconMIMEByExt maps the three asset extensions validateDiscoveryIcons accepts to the
// media type their data URI must carry. It is the same closed set, deliberately: an extension that
// cannot be named here is one the manifest validator would already have refused, and inventing a
// generic fallback would let an unvalidated asset reach an <img> as something the browser sniffs.
var discoveryIconMIMEByExt = map[string]string{
	".svg": "image/svg+xml",
	".png": "image/png",
	".ico": "image/x-icon",
}

// DiscoveryIconMIME returns the media type for a declared icon asset, or "" when the extension is
// not one of the three the manifest allows.
//
// The path is treated as a slash path, matching the manifest: asset paths are written with forward
// slashes regardless of the host OS, and lower-casing the extension keeps "ICON.PNG" from being
// read as an unknown type on a case-insensitive filesystem.
func DiscoveryIconMIME(asset string) string {
	return discoveryIconMIMEByExt[strings.ToLower(path.Ext(strings.ReplaceAll(asset, "\\", "/")))]
}

// DiscoveryIconDataURI encodes one icon asset's bytes as a data URI for the frontend, returning
// ok=false when the extension is unknown or there are no bytes to encode.
//
// base64, never percent-encoding: an SVG is text, and percent-encoding one leaves the plugin's own
// characters — quotes, angle brackets, '#' — in an attribute value, one encoder bug away from
// escaping it. Base64's alphabet cannot terminate the attribute at all.
//
// Encoding lives in the domain because it is pure and it is contract: ADR-014 fixes both the media
// types and the transport form, and a second encoder elsewhere could quietly disagree with this one
// about either. Reading the file is not here — that is the caller's, and the filesystem's, business.
func DiscoveryIconDataURI(asset string, data []byte) (string, bool) {
	mime := DiscoveryIconMIME(asset)
	if mime == "" || len(data) == 0 {
		return "", false
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), true
}
