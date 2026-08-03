package sshconfig

import (
	"reflect"
	"testing"
)

func TestLexLineSyntaxForms(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantKey string
		wantArg []string
		wantOK  bool
	}{
		{name: "simple", raw: "HostName example.com", wantKey: "hostname", wantArg: []string{"example.com"}, wantOK: true},
		{name: "keyword is case insensitive", raw: "hOsTnAmE example.com", wantKey: "hostname", wantArg: []string{"example.com"}, wantOK: true},
		{name: "equals separator", raw: "Port=2222", wantKey: "port", wantArg: []string{"2222"}, wantOK: true},
		{name: "spaced equals separator", raw: "Port = 2222", wantKey: "port", wantArg: []string{"2222"}, wantOK: true},
		{name: "leading whitespace", raw: "    User  deploy", wantKey: "user", wantArg: []string{"deploy"}, wantOK: true},
		{name: "tab separated", raw: "User\tdeploy", wantKey: "user", wantArg: []string{"deploy"}, wantOK: true},
		{name: "multiple args", raw: "Host web-1 web-2 !web-3", wantKey: "host", wantArg: []string{"web-1", "web-2", "!web-3"}, wantOK: true},
		{name: "quoted arg keeps spaces", raw: `IdentityFile "C:\keys\my key"`, wantKey: "identityfile", wantArg: []string{`C:\keys\my key`}, wantOK: true},
		{name: "trailing comment stripped", raw: "Port 22 # the default", wantKey: "port", wantArg: []string{"22"}, wantOK: true},
		{name: "hash inside quotes is data", raw: `IdentityFile "key#1"`, wantKey: "identityfile", wantArg: []string{"key#1"}, wantOK: true},
		{name: "comment only", raw: "  # nothing here", wantOK: false},
		{name: "blank", raw: "   ", wantOK: false},
		{name: "keyword without args", raw: "ForwardAgent", wantKey: "forwardagent", wantArg: nil, wantOK: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := lexLine(tc.raw)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if got.keyword != tc.wantKey {
				t.Errorf("keyword = %q, want %q", got.keyword, tc.wantKey)
			}
			if !reflect.DeepEqual(got.args, tc.wantArg) {
				t.Errorf("args = %#v, want %#v", got.args, tc.wantArg)
			}
		})
	}
}

func TestLexFileToleratesBOMAndCRLF(t *testing.T) {
	content := "\ufeffHost web\r\n  HostName web.example.com\r\n\r\n# comment\r\n  Port 2222\r\n"

	got := lexFile(content)

	want := []string{"host", "hostname", "port"}
	if len(got) != len(want) {
		t.Fatalf("got %d directives, want %d: %#v", len(got), len(want), got)
	}
	for i, keyword := range want {
		if got[i].keyword != keyword {
			t.Errorf("directive %d keyword = %q, want %q", i, got[i].keyword, keyword)
		}
	}
	if got[0].args[0] != "web" {
		t.Errorf("BOM leaked into the first argument: %q", got[0].args[0])
	}
	if got[1].args[0] != "web.example.com" {
		t.Errorf("CR leaked into an argument: %q", got[1].args[0])
	}
}

func TestLexFileRecordsLineNumbers(t *testing.T) {
	got := lexFile("# header\n\nHost web\nPort 22\n")

	if len(got) != 2 {
		t.Fatalf("got %d directives, want 2", len(got))
	}
	if got[0].line != 3 || got[1].line != 4 {
		t.Errorf("lines = %d,%d; want 3,4", got[0].line, got[1].line)
	}
}
