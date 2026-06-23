package plugin

// BundlePort inspects plugin bundle artifacts on disk.
type BundlePort interface {
	HasChecksums(dir string) bool
}
