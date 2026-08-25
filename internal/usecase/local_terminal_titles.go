package usecase

import (
	"slices"
	"strconv"
)

// defaultTerminalCols and defaultTerminalRows are the geometry a shell starts with, before the
// frontend has measured its tile. The first resize from the mounted terminal replaces them; these
// exist so the shell's first prompt is not drawn against a zero-width window.
const (
	defaultTerminalCols = 80
	defaultTerminalRows = 24
)

// disambiguateTitle returns a tab title that no other open terminal is using.
//
// The bare shell name is used while it is free, so the common case - one terminal - reads "bash"
// rather than "bash 1". Numbering starts at 2 and fills gaps, so closing the second of three tabs
// and opening another reuses "bash 2" instead of climbing forever.
func disambiguateTitle(shellName string, inUse []string) string {
	if shellName == "" {
		shellName = "shell"
	}
	if !slices.Contains(inUse, shellName) {
		return shellName
	}
	for n := 2; ; n++ {
		candidate := shellName + " " + strconv.Itoa(n)
		if !slices.Contains(inUse, candidate) {
			return candidate
		}
	}
}
