// Command codeqlgate fails the build on any CodeQL finding that is not in the reviewed list of
// accepted ones.
//
// analyze exits 0 whatever it found, and dismissing an alert in the Security tab changes nothing in
// the SARIF it writes, so a gate reading that SARIF can only tell an accepted finding from a new one
// by a list kept in the repository - where accepting a finding is a diff someone reviews, with the
// reason next to it.
//
// An acceptance names the text of the flagged line rather than its number. Moving the code keeps
// it; editing that line ends it, so changed code is looked at again instead of inheriting a verdict
// given to other code. A data-flow finding is accepted only together with its source line: its sink
// is usually shared - a logging call, a hash helper - and accepting the sink alone would wave
// through the next real secret that reaches it.
//
// Usage: go run ./scripts/codeqlgate --sarif DIR --accepted FILE
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// minReason is the shortest reason accepted: long enough that "false positive" alone is refused.
const minReason = 40

type location struct {
	Path string
	Line int
}

// finding is one CodeQL result; Source is the first step of its data flow, nil for other queries.
type finding struct {
	Rule    string
	At      location
	Source  *location
	Message string
}

// lineRef names a line by its file and its text, trimmed of surrounding whitespace.
type lineRef struct {
	Path string `json:"path"`
	Line string `json:"line"`
}

// acceptance is one reviewed finding the gate lets through.
type acceptance struct {
	Rule   string   `json:"rule"`
	Path   string   `json:"path"`
	Line   string   `json:"line"`
	Source *lineRef `json:"source,omitempty"`
	Reason string   `json:"reason"`
}

func main() {
	sarifDir := flag.String("sarif", "codeql-results", "directory holding the SARIF analyze wrote")
	acceptedFile := flag.String("accepted", ".github/codeql-accepted.json", "reviewed list of accepted findings")
	flag.Parse()
	if err := run(*sarifDir, *acceptedFile); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(sarifDir, acceptedFile string) error {
	accepted, err := loadAccepted(acceptedFile)
	if err != nil {
		return err
	}
	findings, err := loadFindings(sarifDir)
	if err != nil {
		return err
	}
	open, used := triage(findings, accepted, readLine)
	for i, a := range accepted {
		if used[i] {
			fmt.Printf("accepted\t%s\t%s\t%s\n", a.Rule, a.Path, a.Reason)
		} else {
			// Not an error: a pull request run reports only what the pull request touches.
			fmt.Printf("not seen\t%s\t%s\n", a.Rule, a.Path)
		}
	}
	for _, f := range open {
		fmt.Printf("%s\t%s:%d\t%s\n", f.Rule, f.At.Path, f.At.Line, f.Message)
	}
	if len(open) > 0 {
		return fmt.Errorf("CodeQL reported %d finding(s) not in %s", len(open), acceptedFile)
	}
	return nil
}

func loadAccepted(path string) ([]acceptance, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- a path the workflow passes, inside the checkout
	if err != nil {
		return nil, fmt.Errorf("read accepted findings: %w", err)
	}
	var doc struct {
		Accepted []acceptance `json:"accepted"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return doc.Accepted, validate(doc.Accepted)
}

func validate(accepted []acceptance) error {
	for i, a := range accepted {
		switch {
		case a.Rule == "" || a.Path == "" || strings.TrimSpace(a.Line) == "":
			return fmt.Errorf("accepted finding %d: rule, path and line are required", i)
		case a.Source != nil && (a.Source.Path == "" || strings.TrimSpace(a.Source.Line) == ""):
			return fmt.Errorf("accepted finding %d: a source needs both its path and its line", i)
		case len(strings.TrimSpace(a.Reason)) < minReason:
			return fmt.Errorf("accepted finding %d: the reason must say why in at least %d characters", i, minReason)
		}
	}
	return nil
}

// loadFindings reads every SARIF file analyze wrote. None at all means analyze did not finish,
// which must not read as a clean result.
func loadFindings(dir string) ([]finding, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.sarif"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no CodeQL SARIF in %s - analyze did not run to completion", dir)
	}
	var all []finding
	for _, name := range files {
		f, err := os.Open(name) // #nosec G304 -- a file analyze wrote into the given directory
		if err != nil {
			return nil, err
		}
		fs, err := parse(f)
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		all = append(all, fs...)
	}
	return all, nil
}

type sarifLocation struct {
	PhysicalLocation struct {
		ArtifactLocation struct {
			URI string `json:"uri"`
		} `json:"artifactLocation"`
		Region struct {
			StartLine int `json:"startLine"`
		} `json:"region"`
	} `json:"physicalLocation"`
}

func (l sarifLocation) at() location {
	return location{Path: l.PhysicalLocation.ArtifactLocation.URI, Line: l.PhysicalLocation.Region.StartLine}
}

func parse(r io.Reader) ([]finding, error) {
	var doc struct {
		Runs []struct {
			Results []struct {
				RuleID  string `json:"ruleId"`
				Message struct {
					Text string `json:"text"`
				} `json:"message"`
				Locations []sarifLocation `json:"locations"`
				CodeFlows []struct {
					ThreadFlows []struct {
						Locations []struct {
							Location sarifLocation `json:"location"`
						} `json:"locations"`
					} `json:"threadFlows"`
				} `json:"codeFlows"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		return nil, err
	}
	var out []finding
	for _, run := range doc.Runs {
		for _, res := range run.Results {
			f := finding{Rule: res.RuleID, Message: res.Message.Text}
			if len(res.Locations) > 0 {
				f.At = res.Locations[0].at()
			}
			if len(res.CodeFlows) > 0 && len(res.CodeFlows[0].ThreadFlows) > 0 && len(res.CodeFlows[0].ThreadFlows[0].Locations) > 0 {
				src := res.CodeFlows[0].ThreadFlows[0].Locations[0].Location.at()
				f.Source = &src
			}
			out = append(out, f)
		}
	}
	return out, nil
}

// triage splits findings into the open ones and marks which acceptances matched something.
func triage(findings []finding, accepted []acceptance, line func(string, int) (string, error)) ([]finding, []bool) {
	used := make([]bool, len(accepted))
	var open []finding
	for _, f := range findings {
		if i := match(f, accepted, line); i >= 0 {
			used[i] = true
			continue
		}
		open = append(open, f)
	}
	return open, used
}

// match returns the index of the acceptance covering f, or -1.
func match(f finding, accepted []acceptance, line func(string, int) (string, error)) int {
	for i, a := range accepted {
		if a.Rule != f.Rule || a.Path != f.At.Path || (a.Source == nil) != (f.Source == nil) {
			continue
		}
		if !sameLine(f.At, a.Line, line) {
			continue
		}
		if a.Source != nil && (a.Source.Path != f.Source.Path || !sameLine(*f.Source, a.Source.Line, line)) {
			continue
		}
		return i
	}
	return -1
}

// sameLine compares the line at a location with an acceptance's text. A line that cannot be read
// matches nothing, so its finding stays open rather than being accepted blind.
func sameLine(at location, want string, line func(string, int) (string, error)) bool {
	got, err := line(at.Path, at.Line)
	return err == nil && strings.TrimSpace(got) == strings.TrimSpace(want)
}

// readLine reads one 1-based line of a file in the checkout.
func readLine(path string, n int) (string, error) {
	f, err := os.Open(filepath.FromSlash(path)) // #nosec G304 -- a path CodeQL reported inside the checkout
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for i := 1; sc.Scan(); i++ {
		if i == n {
			return sc.Text(), nil
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return "", errors.New("line past the end of the file")
}
