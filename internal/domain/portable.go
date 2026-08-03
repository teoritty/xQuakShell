package domain

// PortableRuntime exposes read-only/write guards for the portable data root (ADR-006).
type PortableRuntime interface {
	RequireWritable() error
	DataRootReadOnly() bool
}

// PortableLayout exposes portable directory paths resolved next to the executable.
type PortableLayout interface {
	DataRoot() string
	TempDir() string
}
