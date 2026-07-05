//go:build !windows

package host

// IsHiddenLocal reports whether a local file entry should be treated as hidden.
func IsHiddenLocal(fullPath, name string) bool {
	return len(name) > 0 && name[0] == '.'
}
