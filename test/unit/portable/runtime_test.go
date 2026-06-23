package portable_test

import (
	"testing"

	"ssh-client/internal/infra/portable"
)

func TestRequireWritableWhenReadOnly(t *testing.T) {
	portable.SetDataRootReadOnly(true)
	t.Cleanup(func() { portable.SetDataRootReadOnly(false) })

	if err := portable.RequireWritable(); err != portable.ErrReadOnlyDataRoot {
		t.Fatalf("expected ErrReadOnlyDataRoot, got %v", err)
	}
}

func TestDataRootReadOnlyFlag(t *testing.T) {
	portable.SetDataRootReadOnly(false)
	if portable.DataRootReadOnly() {
		t.Fatal("expected writable default in test")
	}
	portable.SetDataRootReadOnly(true)
	if !portable.DataRootReadOnly() {
		t.Fatal("expected read-only flag")
	}
	portable.SetDataRootReadOnly(false)
}
