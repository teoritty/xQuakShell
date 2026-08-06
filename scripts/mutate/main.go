// Command mutate runs mutation testing over the packages where it is worth the
// wall time, and ratchets the scores in scripts/mutate/baseline.json.
//
// Line coverage says a test ran the code. Mutation testing says the test would
// have noticed if the code were wrong: gremlins flips a condition, moves a
// boundary, negates a comparison, and reports how many of those the suite
// caught. A test that stays green under mutation is not testing anything.
//
// It is nightly, not per-merge, and the target list is short on purpose. A dry
// run over one small package already takes five minutes because gremlins builds
// a coverage profile for the whole module first; the full matrix is measured in
// hours, which no pull request can wait for.
//
// Usage:
//
//	go run ./scripts/mutate           enforce the recorded scores
//	go run ./scripts/mutate -update   re-record them from this run
//
// Floors can be overridden per target with the environment variable named in
// the table below, which is how a nightly run can be tightened without a
// commit while the change is being judged.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// gremlinsVersion is pinned rather than @latest so a nightly score change means
// the tests changed, not the tool. Keep in step with the Makefile.
const gremlinsVersion = "v0.5.0"

// target is one mutation run. Packages must carry their tests in the same
// package: gremlins runs only the package's own tests unless told to run the
// whole suite, and a target whose tests live elsewhere scores zero for reasons
// that have nothing to do with test quality.
type target struct {
	Label    string
	Packages []string
	MinEnv   string
}

// targets are the pure-logic packages where a mutant maps onto a real defect:
// validation, path safety, protocol rules. Infrastructure that mostly moves
// bytes between two APIs is deliberately absent - there, most mutants are
// either uncatchable without a live server or equivalent to the original.
var targets = []target{
	{Label: "domain/discovery", Packages: []string{"./internal/domain/discovery/"}, MinEnv: "MUTATE_DISCOVERY_MIN"},
}

// The obvious next targets are ./internal/domain/ and ./internal/domain/plugin/,
// which are not listed yet because gremlins v0.5.0 panics part-way through both
// on Windows ("error, this is temporary", engine/executor.go:167) and leaves no
// usable report. Add them once a run completes on the nightly's Linux runner,
// or once the tool is upgraded past the bug - not by pinning a score nobody has
// seen the tool produce.

// score is what one target achieved. Efficacy is the share of covered mutants
// the tests killed; coverage is the share of mutants the tests reach at all.
// Both are recorded: a suite can hold its efficacy while quietly reaching less
// of the code.
type score struct {
	Efficacy float64 `json:"efficacy"`
	Coverage float64 `json:"coverage"`
}

// tolerance absorbs the rounding gremlins applies to its own percentages, so a
// run that changed nothing does not read as drift.
const tolerance = 0.05

func main() {
	update := flag.Bool("update", false, "re-record the baseline from this run")
	flag.Parse()

	if err := run(*update); err != nil {
		fmt.Fprintf(os.Stderr, "mutate: %v\n", err)
		os.Exit(1)
	}
}

func run(update bool) error {
	baseline, err := loadBaseline()
	if err != nil {
		return err
	}

	reportDir, err := os.MkdirTemp("", "xquakshell-mutate-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(reportDir)

	measured := map[string]score{}
	var failures []string

	for _, t := range targets {
		got, err := t.measure(reportDir)
		if err != nil {
			return fmt.Errorf("%s: %w", t.Label, err)
		}
		measured[t.Label] = got

		want, recorded := baseline[t.Label]
		floor := t.floor(want)
		fmt.Printf("%s: efficacy %.1f%%, mutant coverage %.1f%%\n", t.Label, got.Efficacy, got.Coverage)

		if !recorded {
			failures = append(failures, fmt.Sprintf("%s has no recorded score; run `go run ./scripts/mutate -update`", t.Label))
			continue
		}
		failures = append(failures, drift(t.Label, got, floor)...)
	}

	if update {
		return writeBaseline(measured)
	}

	// Every target runs before anything is reported, so one regression cannot
	// hide another behind an early exit.
	if len(failures) > 0 {
		for _, f := range failures {
			fmt.Fprintln(os.Stderr, f)
		}
		return fmt.Errorf("%d mutation gate(s) failed", len(failures))
	}
	return nil
}

// drift fails on a drop and equally on an unrecorded rise. A gate that only
// catches regressions loses every improvement silently, and the recorded score
// stops describing the suite.
func drift(label string, got, want score) []string {
	var out []string
	if got.Efficacy < want.Efficacy-tolerance {
		out = append(out, fmt.Sprintf("%s: efficacy fell to %.1f%% from the recorded %.1f%%; the new or changed tests do not kill what the old ones did", label, got.Efficacy, want.Efficacy))
	}
	if got.Coverage < want.Coverage-tolerance {
		out = append(out, fmt.Sprintf("%s: mutant coverage fell to %.1f%% from the recorded %.1f%%; code was added that no test reaches", label, got.Coverage, want.Coverage))
	}
	if got.Efficacy > want.Efficacy+tolerance || got.Coverage > want.Coverage+tolerance {
		out = append(out, fmt.Sprintf("%s: improved to efficacy %.1f%% / coverage %.1f%% from %.1f%% / %.1f%%; re-record it so the ratchet tightens (`go run ./scripts/mutate -update`)", label, got.Efficacy, got.Coverage, want.Efficacy, want.Coverage))
	}
	return out
}

// floor lets an environment variable raise the recorded efficacy for one run,
// which is how a tightening is trialled on a nightly before it is committed.
func (t target) floor(recorded score) score {
	raw := os.Getenv(t.MinEnv)
	if raw == "" {
		return recorded
	}
	pct, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mutate: %s=%q is not a number, ignoring\n", t.MinEnv, raw)
		return recorded
	}
	recorded.Efficacy = pct
	return recorded
}

func (t target) measure(reportDir string) (score, error) {
	report := filepath.Join(reportDir, strings.NewReplacer("/", "-").Replace(t.Label)+".json")

	// Both flags exist to stop the run reporting a near-zero efficacy for a
	// suite that actually kills almost everything.
	//
	// Gremlins derives each mutant's timeout from how long the unmutated suite
	// took, measured once on an otherwise idle machine. On this module that
	// measurement is dominated by the build rather than the tests, so the
	// default multiple is already tight - and then every mutant runs
	// concurrently against a loaded machine, which is slower still. Left
	// alone on a 16-core host, 33 of 35 mutants time out and the run scores
	// 3% where a serial run scores 100%.
	//
	// The coefficient buys headroom; capping the workers keeps each mutant's
	// run close to the conditions the reference timing was measured under.
	// Neither changes what is being measured, only whether the measurement
	// survives contention.
	args := []string{
		"run", "github.com/go-gremlins/gremlins/cmd/gremlins@" + gremlinsVersion, "unleash",
		"--timeout-coefficient=15",
		"--workers=4",
		"--output=" + report,
	}
	args = append(args, t.Packages...)

	// This program runs via `go run`, so the toolchain is on PATH by construction.
	cmd := exec.Command("go", args...) // #nosec G204 -- arguments come from the target table above, not from user input
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return score{}, fmt.Errorf("gremlins: %w", err)
	}
	return readReport(report)
}

// gremlinsReport is the part of gremlins' machine-readable output this gate
// reads. Mutants are counted here rather than trusting a summary line, because
// the summary is formatted for a human and has changed shape between releases.
type gremlinsReport struct {
	Files []struct {
		Mutations []struct {
			Status string `json:"status"`
		} `json:"mutations"`
	} `json:"files"`
}

func readReport(path string) (score, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is a temp file this program just created
	if err != nil {
		return score{}, fmt.Errorf("read report: %w", err)
	}
	var report gremlinsReport
	if err := json.Unmarshal(data, &report); err != nil {
		return score{}, fmt.Errorf("parse report: %w", err)
	}

	var killed, covered, total int
	for _, file := range report.Files {
		for _, m := range file.Mutations {
			switch m.Status {
			case "KILLED":
				killed++
				covered++
				total++
			case "LIVED", "TIMED OUT":
				// A timeout is not a kill. Counting it as one would hide the
				// failure mode where the timeout is set too low and every
				// mutant "passes" without a test ever finishing.
				covered++
				total++
			case "NOT COVERED":
				total++
			}
			// NOT VIABLE mutants do not compile, so they are not a test
			// failure and must not dilute either percentage.
		}
	}
	if total == 0 {
		return score{}, fmt.Errorf("the report lists no mutants; the target matched nothing")
	}
	return score{
		Efficacy: percent(killed, covered),
		Coverage: percent(covered, total),
	}, nil
}

func percent(part, whole int) float64 {
	if whole == 0 {
		return 0
	}
	return float64(part) / float64(whole) * 100
}

func baselinePath() string {
	return filepath.Join("scripts", "mutate", "baseline.json")
}

func loadBaseline() (map[string]score, error) {
	data, err := os.ReadFile(baselinePath()) // #nosec G304 -- a fixed path inside the repo
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", baselinePath(), err)
	}
	var out map[string]score
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parse %s: %w", baselinePath(), err)
	}
	return out, nil
}

func writeBaseline(scores map[string]score) error {
	data, err := json.MarshalIndent(scores, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(baselinePath(), append(data, '\n'), 0o644); err != nil { // #nosec G306 -- a checked-in config, not a secret
		return err
	}

	labels := make([]string, 0, len(scores))
	for label := range scores {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	for _, label := range labels {
		fmt.Printf("recorded %s: efficacy %.1f%%, coverage %.1f%%\n", label, scores[label].Efficacy, scores[label].Coverage)
	}
	return nil
}
