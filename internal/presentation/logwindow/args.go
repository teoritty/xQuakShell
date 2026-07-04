package logwindow

import (
	"fmt"
	"strings"
)

const (
	flagLogViewer = "--log-viewer"
	flagAddr      = "--addr="
	flagParentPID = "--parent-pid="
)

// IsViewerMode reports whether args start the log viewer subprocess.
func IsViewerMode(args []string) bool {
	for _, a := range args[1:] {
		if a == flagLogViewer {
			return true
		}
	}
	return false
}

// ViewerOptions parsed from subprocess args.
type ViewerOptions struct {
	Addr      string
	ParentPID int
}

// ParseViewerOptions extracts log viewer flags from os.Args.
func ParseViewerOptions(args []string) ViewerOptions {
	opts := ViewerOptions{}
	for _, a := range args[1:] {
		switch {
		case strings.HasPrefix(a, flagAddr):
			opts.Addr = strings.TrimPrefix(a, flagAddr)
		case strings.HasPrefix(a, flagParentPID):
			_, _ = fmt.Sscanf(strings.TrimPrefix(a, flagParentPID), "%d", &opts.ParentPID)
		}
	}
	return opts
}
