package usecase_test

import (
	"testing"

	"xquakshell/internal/usecase"
)

func TestSSHAuthWiring_Enabled(t *testing.T) {
	var nilWiring *usecase.SSHAuthWiring
	if nilWiring.Enabled() {
		t.Fatal("nil wiring should not be enabled")
	}
	empty := &usecase.SSHAuthWiring{}
	if empty.Enabled() {
		t.Fatal("incomplete wiring should not be enabled")
	}
}
