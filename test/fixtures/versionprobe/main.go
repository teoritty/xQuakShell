// Command versionprobe prints the linked-in application version. It exists so tests can verify
// that -ldflags -X against the AppVersion symbol path actually takes effect; a wrong symbol path
// is silently ignored by the linker, so only an end-to-end link proves the path is correct.
package main

import (
	"fmt"

	"xquakshell/internal/presentation/wails"
)

func main() {
	fmt.Print(wails.AppVersion)
}
