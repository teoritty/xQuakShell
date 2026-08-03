package plugin

const (
	// MaxFrameBytes is the maximum NDJSON RPC frame size (256 KiB).
	MaxFrameBytes = 256 << 10
	// MaxReadBytes is the maximum bytes returned per fs.read chunk.
	MaxReadBytes = MaxFrameBytes
	// MaxWriteBytes is the maximum payload per fs.write chunk.
	MaxWriteBytes = MaxFrameBytes
	// MaxFileBytes is the maximum sandboxed file size reachable via chunked I/O.
	MaxFileBytes = 16 << 20

	// MaxBundleUncompressedBytes is the total uncompressed size limit for .xqsp archives.
	MaxBundleUncompressedBytes = 64 << 20
	// MaxBundleEntryCount is the maximum number of files in a plugin bundle.
	MaxBundleEntryCount = 4096
	// MaxBundleEntryUncompressedBytes is the maximum uncompressed size of a single bundle entry.
	MaxBundleEntryUncompressedBytes = MaxFileBytes

	// The Release* limits bound plugin archives fetched from GitHub Releases. They are
	// separate from (and looser than) the Bundle* limits above because a release asset
	// carries a compiled plugin binary, not the source-ish payload of an .xqsp bundle.
	// A checksum is optional on that path, so the archive must be treated as hostile
	// until it is extracted: without these caps a zip/tar bomb fills the user's disk.

	// MaxReleaseAssetBytes is the maximum size of a release asset as downloaded, before
	// any decompression.
	MaxReleaseAssetBytes = 256 << 20
	// MaxReleaseEntryBytes is the maximum uncompressed size of a single entry inside a
	// downloaded release archive.
	MaxReleaseEntryBytes = 128 << 20
	// MaxReleaseUncompressedBytes is the total uncompressed size limit for one release archive.
	MaxReleaseUncompressedBytes = 256 << 20
	// MaxReleaseEntryCount is the maximum number of entries in a release archive.
	MaxReleaseEntryCount = 4096
)
