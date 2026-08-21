package logwindow

import (
	"fmt"
	"strings"
)

const (
	flagLogViewer = "--log-viewer"
	flagAddr      = "--addr="
	flagParentPID = "--parent-pid="
	flagLocale    = "--locale="
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
	// Locale is the language the parent window is displaying.
	//
	// It travels on the command line because the viewer is a separate process with its own
	// WebView: the localStorage mirror the main window writes its language to (i18n/persist.ts)
	// belongs to that window's origin and is not readable here, and the real setting lives in the
	// vault, which this process never unlocks. Empty means English.
	Locale string
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
		case strings.HasPrefix(a, flagLocale):
			opts.Locale = strings.TrimPrefix(a, flagLocale)
		}
	}
	return opts
}
