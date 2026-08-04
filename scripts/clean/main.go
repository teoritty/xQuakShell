// Command clean removes build artifacts after an explicit confirmation.
//
// It exists because build/bin/data is not a build artifact: in portable mode
// the app keeps its vault, audit database and installed plugins there. Wiping
// build/ therefore destroys real user data, irrecoverably on a TRIM-enabled
// SSD. So the deletion is gated behind typing "yes", and the prompt says
// exactly what is about to be lost.
//
// Usage: go run ./scripts/clean [--force]
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// targets are removed wholesale, relative to the repo root.
var targets = []string{"build", "frontend/dist"}

// dataDir is the one path under targets that holds irreplaceable state.
const dataDir = "build/bin/data"

// notable are the files worth naming individually in the warning.
var notable = map[string]string{
	"vault.age":         "credential vault",
	"audit.db":          "audit database",
	"github-repos.json": "plugin repository list",
}

const (
	red   = "\033[1;31m"
	reset = "\033[0m"
)

func main() {
	force := flag.Bool("force", false, "skip the confirmation prompt")
	flag.Parse()

	if err := run(*force); err != nil {
		fmt.Fprintf(os.Stderr, "clean: %v\n", err)
		os.Exit(1)
	}
}

func run(force bool) error {
	if _, err := os.Stat("go.mod"); err != nil {
		return fmt.Errorf("must run from the repository root: %w", err)
	}

	present := existingTargets()
	if len(present) == 0 {
		fmt.Println("Nothing to clean.")
		return nil
	}

	data, err := inspectData()
	if err != nil {
		return err
	}

	printWarning(present, data)

	if !force {
		if err := confirm(data); err != nil {
			return err
		}
	}

	for _, target := range present {
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("remove %s: %w", target, err)
		}
		fmt.Printf("removed %s\n", target)
	}
	return nil
}

func existingTargets() []string {
	var present []string
	for _, target := range targets {
		if _, err := os.Stat(target); err == nil {
			present = append(present, target)
		}
	}
	return present
}

// dataReport summarises what lives under dataDir.
type dataReport struct {
	Exists bool
	Files  int
	Bytes  int64
	Found  []string
}

func inspectData() (dataReport, error) {
	info, err := os.Stat(dataDir)
	if err != nil || !info.IsDir() {
		return dataReport{}, nil
	}

	report := dataReport{Exists: true}
	err = filepath.WalkDir(dataDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		report.Files++
		if fi, statErr := d.Info(); statErr == nil {
			report.Bytes += fi.Size()
		}
		if label, ok := notable[filepath.Base(path)]; ok {
			report.Found = append(report.Found, fmt.Sprintf("%s (%s)", filepath.ToSlash(path), label))
		}
		return nil
	})
	if err != nil {
		return dataReport{}, fmt.Errorf("inspect %s: %w", dataDir, err)
	}
	return report, nil
}

func printWarning(present []string, data dataReport) {
	// Plain ASCII: the banner must survive a legacy console codepage even if
	// the colour escapes do not.
	bar := strings.Repeat("=", 70)

	fmt.Print(colour(red))
	fmt.Println(bar)
	fmt.Println("!!  DESTRUCTIVE OPERATION - THIS PERMANENTLY DELETES FILES  !!")
	fmt.Println(bar)
	fmt.Print(colour(reset))

	fmt.Println("\nAbout to delete:")
	for _, target := range present {
		fmt.Printf("  - %s/\n", target)
	}

	if !data.Exists {
		return
	}

	fmt.Print(colour(red))
	fmt.Printf("\n%s\n", bar)
	fmt.Printf("!!  THIS INCLUDES YOUR LIVE APPLICATION DATA IN %s\n", dataDir)
	fmt.Printf("%s\n", bar)
	fmt.Print(colour(reset))

	fmt.Printf("\n  %d files, %s\n", data.Files, humanBytes(data.Bytes))
	for _, found := range data.Found {
		fmt.Printf("  - %s\n", found)
	}
	fmt.Println("\nThis is the portable-mode data directory, not a build artifact.")
	fmt.Println("There is no undo. On an SSD with TRIM the blocks are released")
	fmt.Println("immediately, so file recovery tools will not bring it back.")
	fmt.Println("Back it up first if you are not certain.")
}

func confirm(data dataReport) error {
	// A non-interactive run must never guess. Fail loudly instead of hanging
	// on a pipe or silently deleting in CI. Piping "yes" is not consent.
	interactive, err := stdinIsTerminal()
	if err != nil {
		return err
	}
	if !interactive {
		return fmt.Errorf("stdin is not a terminal; re-run with FORCE=1 to confirm non-interactively")
	}

	prompt := "\nType yes to continue: "
	if data.Exists {
		prompt = "\nType yes to delete these files, including your application data: "
	}
	fmt.Print(colour(red) + prompt + colour(reset))

	ok, err := readConfirmation(os.Stdin)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("aborted, nothing was deleted")
	}
	return nil
}

func stdinIsTerminal() (bool, error) {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false, err
	}
	return info.Mode()&os.ModeCharDevice != 0, nil
}

// readConfirmation accepts only an exact "yes", ignoring surrounding
// whitespace. Anything else - including "y" and "YES" - is a refusal, so a
// stray keypress cannot destroy the vault.
func readConfirmation(r io.Reader) (bool, error) {
	answer, err := bufio.NewReader(r).ReadString('\n')
	// EOF is normal: it means the answer arrived without a trailing newline,
	// or that nothing was typed at all. Either way it is not a read failure.
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("read confirmation: %w", err)
	}
	return strings.TrimSpace(answer) == "yes", nil
}

// colour returns the escape sequence unless the output is meant to stay plain.
func colour(code string) string {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return ""
	}
	return code
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n/div >= unit && exp < 3 {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}
