package main

import (
	"errors"
	"strings"
	"testing"
)

// source is a fake checkout: path -> lines, 1-based as SARIF counts them.
type source map[string][]string

func (s source) line(path string, n int) (string, error) {
	lines, ok := s[path]
	if !ok || n < 1 || n > len(lines) {
		return "", errors.New("no such line")
	}
	return lines[n-1], nil
}

var tree = source{
	"a/client.go": {"package a", "", "	InsecureSkipVerify: true, // pinned instead"},
	"a/log.go":    {"package a", "	slog.Error(\"failed\", \"err\", err)"},
	"a/src.go":    {"package a", "	ref := b.PasswordID"},
	"a/other.go":  {"package a", "	secret := vault.Password"},
}

const reason = "The pinned key check in VerifyConnection replaces the CA chain here."

func tlsFinding() finding {
	return finding{Rule: "go/disabled-certificate-check", At: location{Path: "a/client.go", Line: 3}, Message: "InsecureSkipVerify"}
}

func flowFinding(src location) finding {
	return finding{Rule: "go/clear-text-logging", At: location{Path: "a/log.go", Line: 2}, Source: &src, Message: "flows to a logging call"}
}

func TestAnAcceptedFindingIsNotOpen(t *testing.T) {
	acc := []acceptance{{Rule: "go/disabled-certificate-check", Path: "a/client.go", Line: "InsecureSkipVerify: true, // pinned instead", Reason: reason}}
	open, used := triage([]finding{tlsFinding()}, acc, tree.line)
	if len(open) != 0 || !used[0] {
		t.Fatalf("open = %v, used = %v; the accepted finding must pass the gate", open, used)
	}
}

// The acceptance names the line's text, not its number: moving the code keeps it, changing the line
// ends it, so an edit to an accepted line is reviewed again instead of inheriting the verdict.
func TestAnAcceptanceFollowsTheLineTextNotItsNumber(t *testing.T) {
	acc := []acceptance{{Rule: "go/disabled-certificate-check", Path: "a/client.go", Line: "InsecureSkipVerify: true, // pinned instead", Reason: reason}}
	moved := source{"a/client.go": {"package a", "", "", "", "	InsecureSkipVerify: true, // pinned instead"}}
	f := tlsFinding()
	f.At.Line = 5
	if open, _ := triage([]finding{f}, acc, moved.line); len(open) != 0 {
		t.Fatal("a moved line lost its acceptance")
	}
	edited := source{"a/client.go": {"package a", "", "	InsecureSkipVerify: true,"}}
	if open, _ := triage([]finding{tlsFinding()}, acc, edited.line); len(open) != 1 {
		t.Fatal("an edited line kept an acceptance made for different code")
	}
}

// A data-flow finding is accepted for one source only. The sink is often shared - a logging call,
// a hash helper - and accepting the sink alone would wave through the next, real secret that
// reaches it.
func TestAFlowFindingIsAcceptedOnlyFromTheSourceNamed(t *testing.T) {
	acc := []acceptance{{Rule: "go/clear-text-logging", Path: "a/log.go", Line: `slog.Error("failed", "err", err)`,
		Source: &lineRef{Path: "a/src.go", Line: "ref := b.PasswordID"}, Reason: reason}}
	if open, _ := triage([]finding{flowFinding(location{Path: "a/src.go", Line: 2})}, acc, tree.line); len(open) != 0 {
		t.Fatal("the accepted flow is still open")
	}
	if open, _ := triage([]finding{flowFinding(location{Path: "a/other.go", Line: 2})}, acc, tree.line); len(open) != 1 {
		t.Fatal("a new source reaching an accepted sink was let through")
	}
	sinkOnly := []acceptance{{Rule: "go/clear-text-logging", Path: "a/log.go", Line: `slog.Error("failed", "err", err)`, Reason: reason}}
	if open, _ := triage([]finding{flowFinding(location{Path: "a/src.go", Line: 2})}, sinkOnly, tree.line); len(open) != 1 {
		t.Fatal("a flow finding was accepted by its sink alone")
	}
}

// Two acceptances of one rule in one file are separate decisions; the second is still found when
// the first is for a different line.
func TestEveryAcceptanceForAFileIsConsidered(t *testing.T) {
	twoLines := source{"a/client.go": {"package a", "	InsecureSkipVerify: true, // first", "	InsecureSkipVerify: true, // second"}}
	acc := []acceptance{
		{Rule: "go/disabled-certificate-check", Path: "a/client.go", Line: "InsecureSkipVerify: true, // first", Reason: reason},
		{Rule: "go/disabled-certificate-check", Path: "a/client.go", Line: "InsecureSkipVerify: true, // second", Reason: reason},
	}
	f := tlsFinding()
	f.At.Line = 3
	open, used := triage([]finding{f}, acc, twoLines.line)
	if len(open) != 0 || used[0] || !used[1] {
		t.Fatalf("open = %v, used = %v; the second acceptance is the one for line 3", open, used)
	}
}

func TestAnotherRuleOnAnAcceptedLineIsOpen(t *testing.T) {
	acc := []acceptance{{Rule: "go/disabled-certificate-check", Path: "a/client.go", Line: "InsecureSkipVerify: true, // pinned instead", Reason: reason}}
	f := tlsFinding()
	f.Rule = "go/insecure-tls"
	if open, _ := triage([]finding{f}, acc, tree.line); len(open) != 1 {
		t.Fatal("an acceptance covered a rule it does not name")
	}
}

// An acceptance is a reviewed decision; one without a reason that says why is refused before it can
// hide anything.
func TestAnAcceptanceNeedsAReason(t *testing.T) {
	for _, a := range []acceptance{
		{Rule: "r", Path: "p", Line: "l", Reason: "false positive"},
		{Rule: "", Path: "p", Line: "l", Reason: reason},
		{Rule: "r", Path: "p", Line: "  ", Reason: reason},
		{Rule: "r", Path: "p", Line: "l", Source: &lineRef{Path: "s"}, Reason: reason},
	} {
		if err := validate([]acceptance{a}); err == nil {
			t.Errorf("%+v accepted; rule, path, line, a whole source and a reason of 40+ characters are required", a)
		}
	}
	if err := validate([]acceptance{{Rule: "r", Path: "p", Line: "l", Reason: reason}}); err != nil {
		t.Fatal(err)
	}
}

func TestParseReadsTheLocationAndTheFlowSource(t *testing.T) {
	const sarif = `{"runs":[{"results":[
	 {"ruleId":"go/clear-text-logging","message":{"text":"m"},
	  "locations":[{"physicalLocation":{"artifactLocation":{"uri":"a/log.go"},"region":{"startLine":2}}}],
	  "codeFlows":[{"threadFlows":[{"locations":[
	    {"location":{"physicalLocation":{"artifactLocation":{"uri":"a/src.go"},"region":{"startLine":2}}}},
	    {"location":{"physicalLocation":{"artifactLocation":{"uri":"a/log.go"},"region":{"startLine":2}}}}]}]}]},
	 {"ruleId":"go/disabled-certificate-check","message":{"text":"t"},
	  "locations":[{"physicalLocation":{"artifactLocation":{"uri":"a/client.go"},"region":{"startLine":3}}}]}]}]}`
	fs, err := parse(strings.NewReader(sarif))
	if err != nil || len(fs) != 2 {
		t.Fatalf("findings = %+v, %v", fs, err)
	}
	if fs[0].Source == nil || *fs[0].Source != (location{Path: "a/src.go", Line: 2}) || fs[0].At != (location{Path: "a/log.go", Line: 2}) {
		t.Fatalf("flow = %+v; the first step of the flow is its source", fs[0])
	}
	if fs[1].Source != nil {
		t.Fatalf("a finding without a flow has no source: %+v", fs[1])
	}
}
