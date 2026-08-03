package portable

// RuntimeAdapter implements domain.PortableRuntime using the process-wide portable flags.
type RuntimeAdapter struct{}

// LayoutAdapter implements domain.PortableLayout for a Paths instance.
type LayoutAdapter struct {
	Paths *Paths
}

// NewRuntimeAdapter returns a portable runtime guard adapter.
func NewRuntimeAdapter() RuntimeAdapter {
	return RuntimeAdapter{}
}

// RequireWritable implements domain.PortableRuntime.
func (RuntimeAdapter) RequireWritable() error {
	return RequireWritable()
}

// DataRootReadOnly implements domain.PortableRuntime.
func (RuntimeAdapter) DataRootReadOnly() bool {
	return DataRootReadOnly()
}

// NewLayoutAdapter wraps portable paths for injection into usecase/presentation.
func NewLayoutAdapter(p *Paths) LayoutAdapter {
	if p == nil {
		p = Default
	}
	return LayoutAdapter{Paths: p}
}

// DataRoot implements domain.PortableLayout.
func (a LayoutAdapter) DataRoot() string {
	return a.Paths.DataRoot()
}

// TempDir implements domain.PortableLayout.
func (a LayoutAdapter) TempDir() string {
	return a.Paths.TempDir()
}
