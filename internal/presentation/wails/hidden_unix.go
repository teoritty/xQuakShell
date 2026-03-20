//go:build !windows

package wails

func isHiddenLocal(fullPath, name string) bool {
	return len(name) > 0 && name[0] == '.'
}
