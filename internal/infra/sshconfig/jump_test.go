package sshconfig

import (
	"testing"

	"xquakshell/internal/domain"
)

func TestParseJumpSpec(t *testing.T) {
	tests := []struct {
		name   string
		spec   string
		want   jumpSpec
		wantOK bool
	}{
		{name: "host only", spec: "bastion", want: jumpSpec{alias: "bastion"}, wantOK: true},
		{name: "user and host", spec: "admin@bastion", want: jumpSpec{user: "admin", alias: "bastion"}, wantOK: true},
		{name: "host and port", spec: "bastion:2222", want: jumpSpec{alias: "bastion", port: 2222}, wantOK: true},
		{name: "user host port", spec: "admin@bastion:2222", want: jumpSpec{user: "admin", alias: "bastion", port: 2222}, wantOK: true},
		{name: "bracketed ipv6", spec: "[2001:db8::1]", want: jumpSpec{alias: "2001:db8::1"}, wantOK: true},
		{name: "bracketed ipv6 with port", spec: "[2001:db8::1]:2222", want: jumpSpec{alias: "2001:db8::1", port: 2222}, wantOK: true},
		{name: "bare ipv6 has no port", spec: "2001:db8::1", want: jumpSpec{alias: "2001:db8::1"}, wantOK: true},
		{name: "user with bracketed ipv6", spec: "admin@[2001:db8::1]:22", want: jumpSpec{user: "admin", alias: "2001:db8::1", port: 22}, wantOK: true},
		{name: "empty", spec: "", wantOK: false},
		{name: "user only", spec: "admin@", wantOK: false},
		{name: "invalid port", spec: "bastion:99999", wantOK: false},
		{name: "non numeric port", spec: "bastion:ssh", wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseJumpSpec(tc.spec)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseResolvesJumpHopThroughItsOwnBlock(t *testing.T) {
	fakeHome(t)
	path := writeFile(t, t.TempDir(), "config", `
Host bastion
    HostName bastion.example.com
    User jump
    Port 2222

Host internal
    HostName 10.0.0.5
    ProxyJump bastion
`)

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	hops := hostByAlias(t, result, "internal").JumpHops
	if len(hops) != 1 {
		t.Fatalf("got %d hops, want 1", len(hops))
	}
	want := domain.SSHConfigHop{Alias: "bastion", HostName: "bastion.example.com", Port: 2222, User: "jump"}
	if hops[0].Alias != want.Alias || hops[0].HostName != want.HostName || hops[0].Port != want.Port || hops[0].User != want.User {
		t.Errorf("hop = %+v, want %+v — a bare ProxyJump must inherit the hop's own block", hops[0], want)
	}
}

func TestParseInlineJumpValuesOverrideTheBlock(t *testing.T) {
	fakeHome(t)
	path := writeFile(t, t.TempDir(), "config", `
Host bastion
    HostName bastion.example.com
    User jump
    Port 2222

Host internal
    ProxyJump admin@bastion:2200
`)

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	hop := hostByAlias(t, result, "internal").JumpHops[0]
	if hop.User != "admin" || hop.Port != 2200 {
		t.Errorf("hop = %+v, want the inline user/port to win", hop)
	}
	if hop.HostName != "bastion.example.com" {
		t.Errorf("hop hostname = %q, want the block's HostName to still apply", hop.HostName)
	}
}

func TestParseMultiHopJumpChainKeepsOrder(t *testing.T) {
	fakeHome(t)
	path := writeFile(t, t.TempDir(), "config", `
Host target
    ProxyJump first,second
`)

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	hops := hostByAlias(t, result, "target").JumpHops
	if len(hops) != 2 || hops[0].Alias != "first" || hops[1].Alias != "second" {
		t.Fatalf("hops = %+v, want [first second] in traversal order", hops)
	}
}

func TestParseNestedJumpHopComesFirst(t *testing.T) {
	fakeHome(t)
	path := writeFile(t, t.TempDir(), "config", `
Host outer
    HostName outer.example.com

Host inner
    HostName inner.example.com
    ProxyJump outer

Host target
    ProxyJump inner
`)

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	hops := hostByAlias(t, result, "target").JumpHops
	if len(hops) != 2 || hops[0].Alias != "outer" || hops[1].Alias != "inner" {
		t.Fatalf("hops = %+v, want [outer inner]: the hop's own jump is traversed first", hops)
	}
}

func TestParseJumpNoneDisablesTheChain(t *testing.T) {
	fakeHome(t)
	path := writeFile(t, t.TempDir(), "config", "Host target\n  ProxyJump none\n")

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if hops := hostByAlias(t, result, "target").JumpHops; len(hops) != 0 {
		t.Errorf("hops = %+v, want none", hops)
	}
}

func TestParseSurvivesJumpCycle(t *testing.T) {
	fakeHome(t)
	path := writeFile(t, t.TempDir(), "config", `
Host a
    ProxyJump b

Host b
    ProxyJump a
`)

	done := make(chan domain.SSHConfigParseResult, 1)
	go func() {
		result, err := Parse(path)
		if err != nil {
			t.Errorf("Parse: %v", err)
		}
		done <- result
	}()

	select {
	case result := <-done:
		host := hostByAlias(t, result, "a")
		if len(host.JumpHops) != 1 || host.JumpHops[0].Alias != "b" {
			t.Errorf("hops = %+v, want the cycle cut after one hop", host.JumpHops)
		}
		if !hasNotice(result, domain.SSHConfigNoticeJumpHostUnresolved, "b") {
			t.Errorf("a cut jump cycle must be reported; notices = %+v", result.Notices)
		}
	case <-timeoutAfterSeconds(10):
		t.Fatal("Parse did not terminate on a cyclic ProxyJump")
	}
}

func TestParseReportsUnparsableJumpSpec(t *testing.T) {
	fakeHome(t)
	path := writeFile(t, t.TempDir(), "config", "Host target\n  ProxyJump bastion:notaport\n")

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if hops := hostByAlias(t, result, "target").JumpHops; len(hops) != 0 {
		t.Errorf("hops = %+v, want none for an unparsable spec", hops)
	}
	if !hasNotice(result, domain.SSHConfigNoticeJumpHostUnresolved, "target") {
		t.Errorf("an unparsable ProxyJump must be reported; notices = %+v", result.Notices)
	}
}
