//go:build !windows

package main

import (
	"fmt"
	"os"
)

// Linux and macOS builds are normally started from a terminal or by a desktop entry that keeps the
// journal, so stderr reaches someone. A GUI dialog here would mean depending on GTK before the app
// has proven it can initialise one.
func displayStartupFailure(title, body string) {
	fmt.Fprintf(os.Stderr, "%s\n\n%s\n", title, body)
}
