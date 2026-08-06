package plugin

import "strings"

// ReleaseAssetKind tells apart the two shapes a plugin can be published in.
type ReleaseAssetKind string

const (
	// ReleaseAssetBundle is an .xqsp archive: the manifest, the binary, the ui/ tree and the
	// author's SHA256SUMS, exactly as they were packaged. It is the only shape that can carry a
	// plugin's UI, and the only one whose checksums are a statement by someone other than us.
	ReleaseAssetBundle ReleaseAssetKind = "bundle"
	// ReleaseAssetBinary is a bare executable, possibly inside a .zip or .tar.gz. It carries the
	// plugin's code and nothing else.
	ReleaseAssetBinary ReleaseAssetKind = "binary"
)

// BundleAssetSuffix is the file extension of a packaged plugin bundle.
const BundleAssetSuffix = ".xqsp"

// ClassifyReleaseAsset reports which shape a release asset file name carries.
func ClassifyReleaseAsset(assetName string) ReleaseAssetKind {
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(assetName)), BundleAssetSuffix) {
		return ReleaseAssetBundle
	}
	return ReleaseAssetBinary
}

// AssetDownloadRequest names one release asset to fetch.
type AssetDownloadRequest struct {
	Owner string
	Repo  string
	Tag   string
	// AssetName is the release asset file name, as listed by the GitHub API.
	AssetName string
	// ExpectedChecksum is this asset's entry in the release-level SHA256SUMS. Empty means the
	// release published none and the download cannot be verified against it.
	ExpectedChecksum string
	// EntryName is the manifest's engine.entry. It is what identifies the plugin binary inside a
	// release archive; without it an archive offers no way to tell the binary from a README.
	EntryName string
}

// DownloadedAsset is a release asset that now exists on local disk.
//
// Path stays valid only until the cleanup function returned alongside it runs.
type DownloadedAsset struct {
	// Path is the bundle file for ReleaseAssetBundle, and the plugin executable — extracted from
	// the archive when the asset was one — for ReleaseAssetBinary.
	Path string
	Kind ReleaseAssetKind
	// AssetName is the original release asset name, kept for error messages.
	AssetName string
}

// StagedPlugin is a plugin tree prepared for install, with the manifest that tree actually
// contains. The distinction matters: for a bundle that manifest is the author's plugin.json, not
// the xqsp.json the repository advertised, and the install decides against the former.
type StagedPlugin struct {
	Dir      string
	Manifest Manifest
}
