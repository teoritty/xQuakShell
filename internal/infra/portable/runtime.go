package portable

import (
	"errors"
	"sync"
)

// ErrReadOnlyDataRoot is returned when a write operation targets a read-only portable data root.
var ErrReadOnlyDataRoot = errors.New("portable data root is read-only")

var (
	runtimeMu        sync.RWMutex
	dataRootReadOnly bool
)

// SetDataRootReadOnly records whether the portable data root rejects writes.
func SetDataRootReadOnly(readOnly bool) {
	runtimeMu.Lock()
	dataRootReadOnly = readOnly
	runtimeMu.Unlock()
}

// DataRootReadOnly reports whether the portable data root is read-only.
func DataRootReadOnly() bool {
	runtimeMu.RLock()
	defer runtimeMu.RUnlock()
	return dataRootReadOnly
}

// InitRuntime detects read-only media and records runtime flags.
func InitRuntime(p *Paths) error {
	readOnly, err := DetectReadOnlyDataRoot(p)
	if err != nil {
		return err
	}
	SetDataRootReadOnly(readOnly)
	return nil
}
