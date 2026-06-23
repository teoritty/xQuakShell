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

	// MaxBundleUncompressedBytes is the total uncompressed size limit for .xqs-plugin archives.
	MaxBundleUncompressedBytes = 64 << 20
	// MaxBundleEntryCount is the maximum number of files in a plugin bundle.
	MaxBundleEntryCount = 4096
	// MaxBundleEntryUncompressedBytes is the maximum uncompressed size of a single bundle entry.
	MaxBundleEntryUncompressedBytes = MaxFileBytes
)
