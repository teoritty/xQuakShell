package portable

// RequireWritable returns ErrReadOnlyDataRoot when the portable data root rejects writes.
func RequireWritable() error {
	if DataRootReadOnly() {
		return ErrReadOnlyDataRoot
	}
	return nil
}
