package sshconfig

import (
	"reflect"
	"testing"
)

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		pattern string
		name    string
		want    bool
	}{
		{pattern: "web", name: "web", want: true},
		{pattern: "web", name: "web-1", want: false},
		{pattern: "*", name: "anything", want: true},
		{pattern: "web-*", name: "web-1", want: true},
		{pattern: "web-*", name: "db-1", want: false},
		{pattern: "*.example.com", name: "host.example.com", want: true},
		{pattern: "*.example.com", name: "example.com", want: false},
		{pattern: "web-?", name: "web-1", want: true},
		{pattern: "web-?", name: "web-12", want: false},
		{pattern: "*-*-*", name: "a-b-c", want: true},
		{pattern: "a*b*c", name: "aXXbYYc", want: true},
		{pattern: "a*b*c", name: "aXXbYY", want: false},
		// Backtracking: the first '*' must give back characters so the
		// literal tail can match.
		{pattern: "*abc", name: "zzabcabc", want: true},
		{pattern: "WEB-*", name: "web-1", want: true},
	}

	for _, tc := range tests {
		t.Run(tc.pattern+"/"+tc.name, func(t *testing.T) {
			if got := matchPattern(tc.pattern, tc.name); got != tc.want {
				t.Errorf("matchPattern(%q, %q) = %v, want %v", tc.pattern, tc.name, got, tc.want)
			}
		})
	}
}

func TestBlockMatchesHonoursNegation(t *testing.T) {
	b := block{patterns: toPatterns([]string{"web-*", "!web-staging"})}

	if !b.matches("web-1") {
		t.Error("web-1 should match web-*")
	}
	if b.matches("web-staging") {
		t.Error("web-staging is negated and must not match")
	}
	if b.matches("db-1") {
		t.Error("db-1 matches no positive pattern")
	}
}

func TestMatchBlockNeverMatches(t *testing.T) {
	b := block{isMatch: true, patterns: toPatterns([]string{"*"})}

	if b.matches("web") {
		t.Error("Match blocks must not contribute settings")
	}
}

func TestBlockClassification(t *testing.T) {
	tests := []struct {
		name         string
		patterns     []string
		wantDefaults bool
		wantAliases  []string
	}{
		{name: "wildcard only", patterns: []string{"*"}, wantDefaults: true},
		{name: "negation only", patterns: []string{"!web"}, wantDefaults: true},
		{name: "concrete", patterns: []string{"web"}, wantAliases: []string{"web"}},
		{name: "mixed", patterns: []string{"web-*", "bastion"}, wantAliases: []string{"bastion"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := block{patterns: toPatterns(tc.patterns)}
			if got := b.isDefaultsOnly(); got != tc.wantDefaults {
				t.Errorf("isDefaultsOnly = %v, want %v", got, tc.wantDefaults)
			}
			if got := b.concreteAliases(); !reflect.DeepEqual(got, tc.wantAliases) {
				t.Errorf("concreteAliases = %#v, want %#v", got, tc.wantAliases)
			}
		})
	}
}

func TestGroupBlocksTreatsPreambleAsGlobalDefaults(t *testing.T) {
	blocks := groupBlocks(lexFile("Port 2222\nHost web\n  HostName web.example.com\n"))

	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(blocks))
	}
	if !blocks[0].isDefaultsOnly() {
		t.Error("directives before the first Host must behave as global defaults")
	}
	if !blocks[0].matches("web") {
		t.Error("the preamble block must apply to every host")
	}
	if got := blocks[1].concreteAliases(); !reflect.DeepEqual(got, []string{"web"}) {
		t.Errorf("second block aliases = %#v, want [web]", got)
	}
}
