package plugin_test

import (
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

func TestParseSemver(t *testing.T) {
	ok := []struct {
		in            string
		maj, min, pat int
		pre           string
	}{
		{"1.0.0", 1, 0, 0, ""},
		{"0.2.0", 0, 2, 0, ""},
		{"12.34.56", 12, 34, 56, ""},
		{"1.1.0-rc1", 1, 1, 0, "rc1"},
		{"0.2.0-dev", 0, 2, 0, "dev"},
		{" 1.0.0 ", 1, 0, 0, ""},
	}
	for _, tc := range ok {
		v, err := domainplugin.ParseSemver(tc.in)
		if err != nil {
			t.Fatalf("ParseSemver(%q) unexpected error: %v", tc.in, err)
		}
		if v.Major != tc.maj || v.Minor != tc.min || v.Patch != tc.pat || v.Pre != tc.pre {
			t.Fatalf("ParseSemver(%q) = %+v, want %d.%d.%d-%q", tc.in, v, tc.maj, tc.min, tc.pat, tc.pre)
		}
	}

	bad := []string{
		"", "1", "1.0", "1.0.0.0", "v1.0.0", "1.0.x", "01.0.0", "1.00.0",
		"1.0.-1", "1.0.0-", "-1.0.0", "1.0.0+build", "latest",
	}
	for _, in := range bad {
		if _, err := domainplugin.ParseSemver(in); err == nil {
			t.Fatalf("ParseSemver(%q) expected error, got nil", in)
		}
	}
}

func TestSatisfies(t *testing.T) {
	tests := []struct {
		have, want string
		ok         bool
	}{
		{"1.0.0", "1.0.0", true},     // exact
		{"1.3.0", "1.2.0", true},     // higher minor
		{"1.2.5", "1.2.0", true},     // higher patch, same minor
		{"1.2.0", "1.2.9", true},     // patch never gates
		{"1.1.0", "1.2.0", false},    // host minor too low
		{"2.0.0", "1.0.0", false},    // newer major does NOT satisfy (breaking)
		{"1.0.0", "2.0.0", false},    // host too old on major
		{"1.1.0-rc1", "1.0.0", true}, // pre-release host satisfies release requirement
	}
	for _, tc := range tests {
		have, err := domainplugin.ParseSemver(tc.have)
		if err != nil {
			t.Fatalf("parse have %q: %v", tc.have, err)
		}
		want, err := domainplugin.ParseSemver(tc.want)
		if err != nil {
			t.Fatalf("parse want %q: %v", tc.want, err)
		}
		if got := domainplugin.Satisfies(have, want); got != tc.ok {
			t.Fatalf("Satisfies(%q, %q) = %v, want %v", tc.have, tc.want, got, tc.ok)
		}
	}
}

func TestSemverString(t *testing.T) {
	for _, in := range []string{"1.0.0", "1.2.3-rc1"} {
		v, err := domainplugin.ParseSemver(in)
		if err != nil {
			t.Fatalf("parse %q: %v", in, err)
		}
		if v.String() != in {
			t.Fatalf("String() = %q, want %q", v.String(), in)
		}
	}
}
