// Command coverage enforces per-package coverage floors for the plugin stack.
//
// It replaces scripts/check-plugin-coverage.sh: bash is not dependable on
// Windows, where the `bash` on PATH is usually the WSL shim, a separate OS
// that cannot see the Go toolchain. Running the gate through `go run` needs
// nothing beyond the toolchain the gate is already measuring.
//
// Usage: go run ./scripts/coverage
// Floors can be overridden with DOMAIN_MIN, INFRA_MIN and USECASE_MIN.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type gate struct {
	Label    string
	Packages []string
	CoverPkg []string
	MinEnv   string
	MinPct   float64
}

var gates = []gate{
	{
		Label:    "domain/plugin",
		Packages: []string{"./internal/domain/plugin/", "./test/unit/plugin/"},
		CoverPkg: []string{"xquakshell/internal/domain/plugin"},
		MinEnv:   "DOMAIN_MIN",
		MinPct:   80,
	},
	{
		Label:    "infra/plugin",
		Packages: []string{"./internal/infra/plugin/", "./internal/infra/plugin/ipc/", "./test/unit/plugin/"},
		CoverPkg: []string{
			"xquakshell/internal/infra/plugin",
			"xquakshell/internal/infra/plugin/assets",
			"xquakshell/internal/infra/plugin/bundle",
			"xquakshell/internal/infra/plugin/capability",
			"xquakshell/internal/infra/plugin/ipc",
			"xquakshell/internal/infra/plugin/lifecycle",
		},
		MinEnv: "INFRA_MIN",
		MinPct: 60,
	},
	{
		Label:    "usecase plugin",
		Packages: []string{"./internal/usecase/", "./test/unit/plugin/"},
		CoverPkg: []string{"xquakshell/internal/usecase"},
		MinEnv:   "USECASE_MIN",
		MinPct:   50,
	},
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "coverage: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Profiles are scratch data; the shell version left them in the repo root.
	profileDir, err := os.MkdirTemp("", "xquakshell-coverage-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(profileDir)

	var failures []string
	for _, g := range gates {
		min, err := g.minimum()
		if err != nil {
			return err
		}

		profile := filepath.Join(profileDir, strings.NewReplacer("/", "-", " ", "-").Replace(g.Label)+".out")
		if err := g.runTests(profile); err != nil {
			return fmt.Errorf("%s tests failed: %w", g.Label, err)
		}

		pct, err := totalCoverage(profile)
		if err != nil {
			return fmt.Errorf("%s: %w", g.Label, err)
		}

		fmt.Printf("%s coverage: %.1f%% (min %.1f%%)\n", g.Label, pct, min)
		if pct < min {
			failures = append(failures, fmt.Sprintf("%s: %.1f%% is below the %.1f%% floor", g.Label, pct, min))
		}
	}

	// Every gate runs before reporting, so one regression does not hide another.
	if len(failures) > 0 {
		for _, f := range failures {
			fmt.Fprintln(os.Stderr, f)
		}
		return fmt.Errorf("%d coverage gate(s) failed", len(failures))
	}
	return nil
}

func (g gate) minimum() (float64, error) {
	raw := os.Getenv(g.MinEnv)
	if raw == "" {
		return g.MinPct, nil
	}
	pct, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("%s=%q is not a number: %w", g.MinEnv, raw, err)
	}
	return pct, nil
}

func (g gate) runTests(profile string) error {
	args := append([]string{"test"}, g.Packages...)
	args = append(args,
		"-coverprofile="+profile,
		"-covermode=atomic",
		"-coverpkg="+strings.Join(g.CoverPkg, ","),
	)

	// This program runs via `go run`, so the toolchain is on PATH by construction.
	cmd := exec.Command("go", args...) // #nosec G204 -- arguments come from the gate table above, not from user input
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// totalCoverage reads the "total:" line of `go tool cover -func`, whose last
// field is the percentage.
func totalCoverage(profile string) (float64, error) {
	out, err := exec.Command("go", "tool", "cover", "-func="+profile).Output() // #nosec G204 -- profile is a path this program just created
	if err != nil {
		return 0, fmt.Errorf("go tool cover: %w", err)
	}

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "total:" {
			continue
		}
		pct := strings.TrimSuffix(fields[len(fields)-1], "%")
		value, err := strconv.ParseFloat(pct, 64)
		if err != nil {
			return 0, fmt.Errorf("unparsable total %q: %w", fields[len(fields)-1], err)
		}
		return value, nil
	}
	return 0, fmt.Errorf("no total line in coverage profile %s", profile)
}
