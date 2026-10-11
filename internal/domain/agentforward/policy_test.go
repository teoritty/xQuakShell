package agentforward

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"
)

// --- bindings ---

func binding(sid string, hostKey string) Binding {
	return Binding{SessionID: []byte(sid), HostKey: []byte(hostKey), HostKeyFingerprint: "SHA256:" + hostKey}
}

func TestBindingsRecordAndFindASession(t *testing.T) {
	var b Bindings
	if err := b.Add(binding("sid-1", "host-b")); err != nil {
		t.Fatal(err)
	}
	got, ok := b.Lookup([]byte("sid-1"))
	if !ok || got.HostKeyFingerprint != "SHA256:host-b" {
		t.Fatalf("Lookup = %+v, %v; a recorded binding must be found", got, ok)
	}
	if _, ok := b.Lookup([]byte("sid-2")); ok {
		t.Error("an unbound session must not be found")
	}
}

func TestBindingsRefuseRebindingASessionToAnotherHost(t *testing.T) {
	var b Bindings
	if err := b.Add(binding("sid-1", "host-b")); err != nil {
		t.Fatal(err)
	}
	if err := b.Add(binding("sid-1", "host-b")); err != nil {
		t.Errorf("rebinding to the same host is a retry and must be accepted: %v", err)
	}
	if err := b.Add(binding("sid-1", "host-evil")); !errors.Is(err, ErrBindingRefused) {
		t.Fatalf("err = %v; one session id claimed by two hosts is a forgery and must be refused", err)
	}
	if got, _ := b.Lookup([]byte("sid-1")); got.HostKeyFingerprint != "SHA256:host-b" {
		t.Error("a refused rebind must leave the original binding in place")
	}
}

func TestBindingsRefuseIncompleteAndExcessBindings(t *testing.T) {
	var b Bindings
	for name, nb := range map[string]Binding{
		"no session":     {HostKey: []byte("k"), HostKeyFingerprint: "f"},
		"no host key":    {SessionID: []byte("s"), HostKeyFingerprint: "f"},
		"no fingerprint": {SessionID: []byte("s"), HostKey: []byte("k")},
		"long session":   {SessionID: bytes.Repeat([]byte{1}, MaxSessionIDLen+1), HostKey: []byte("k"), HostKeyFingerprint: "f"},
	} {
		if err := b.Add(nb); !errors.Is(err, ErrBindingRefused) {
			t.Errorf("%s: err = %v, want ErrBindingRefused", name, err)
		}
	}
	for i := range MaxBindingsPerChannel {
		if err := b.Add(binding(fmt.Sprintf("sid-%d", i), "h")); err != nil {
			t.Fatalf("binding %d of %d refused: %v", i+1, MaxBindingsPerChannel, err)
		}
	}
	if err := b.Add(binding("one-too-many", "h")); !errors.Is(err, ErrBindingRefused) {
		t.Fatalf("binding %d accepted; the table must stay bounded", MaxBindingsPerChannel+1)
	}
}

func TestBindingsDoNotShareMemoryWithTheCaller(t *testing.T) {
	var b Bindings
	nb := binding("sid-1", "host-b")
	if err := b.Add(nb); err != nil {
		t.Fatal(err)
	}
	nb.HostKey[0] = 'X'
	got, _ := b.Lookup([]byte("sid-1"))
	got.SessionID[0] = 'X'
	again, ok := b.Lookup([]byte("sid-1"))
	if !ok || again.HostKey[0] != 'h' {
		t.Fatal("a stored binding must not change when the caller's slices do")
	}
}

func TestDestinationIsVerifiedOnlyForABoundSession(t *testing.T) {
	var b Bindings
	if err := b.Add(Binding{SessionID: testSessionID, HostKey: []byte("host-b"), HostKeyFingerprint: "fp-b"}); err != nil {
		t.Fatal(err)
	}
	got, verified, err := Destination(UserAuthRequest{SessionID: testSessionID}, &b)
	if err != nil || !verified || got.HostKeyFingerprint != "fp-b" {
		t.Fatalf("Destination = %+v, %v, %v; want the bound host", got, verified, err)
	}
	if _, verified, err := Destination(UserAuthRequest{SessionID: []byte("other")}, &b); verified || err != nil {
		t.Errorf("an unbound session must be unverified, not an error: %v %v", verified, err)
	}
	if _, verified, _ := Destination(UserAuthRequest{SessionID: testSessionID}, nil); verified {
		t.Error("a channel with no bindings must not verify anything")
	}
}

func TestDestinationRefusesAHostboundLoginForAnotherHost(t *testing.T) {
	var b Bindings
	if err := b.Add(Binding{SessionID: testSessionID, HostKey: []byte("host-b"), HostKeyFingerprint: "fp-b"}); err != nil {
		t.Fatal(err)
	}
	req := UserAuthRequest{SessionID: testSessionID, HostKey: []byte("host-evil")}
	if _, _, err := Destination(req, &b); !errors.Is(err, ErrHostKeyMismatch) {
		t.Fatalf("err = %v; the dialog would show host-b while the signature commits to host-evil", err)
	}
	req.HostKey = []byte("host-b")
	if _, verified, err := Destination(req, &b); err != nil || !verified {
		t.Errorf("a hostbound login agreeing with its binding must verify: %v", err)
	}
}

// --- approval window ---

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newClock() *fakeClock { return &fakeClock{t: time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)} }

var grant = Grant{SessionID: "s1", IdentityID: "k1", Destination: "fp-b"}

func TestApprovalWindowCoversExactlyWhatWasGranted(t *testing.T) {
	clock := newClock()
	w := NewApprovalWindow(clock.now)
	if err := w.Allow(grant, 15*time.Minute); err != nil {
		t.Fatal(err)
	}
	if !w.Allows(grant) {
		t.Fatal("a fresh grant must allow")
	}
	for name, g := range map[string]Grant{
		"other session":     {SessionID: "s2", IdentityID: "k1", Destination: "fp-b"},
		"other key":         {SessionID: "s1", IdentityID: "k2", Destination: "fp-b"},
		"other destination": {SessionID: "s1", IdentityID: "k1", Destination: "fp-evil"},
		"unverified":        {SessionID: "s1", IdentityID: "k1"},
	} {
		if w.Allows(g) {
			t.Errorf("%s: a window granted for one session, key and host must not cover another", name)
		}
	}
}

func TestApprovalWindowEndsAtItsExpiry(t *testing.T) {
	clock := newClock()
	w := NewApprovalWindow(clock.now)
	if err := w.Allow(grant, time.Minute); err != nil {
		t.Fatal(err)
	}
	clock.advance(time.Minute - time.Nanosecond)
	if !w.Allows(grant) {
		t.Fatal("a grant must hold until its expiry instant")
	}
	clock.advance(time.Nanosecond)
	if w.Allows(grant) {
		t.Fatal("a grant must not hold at its expiry instant")
	}
}

func TestApprovalWindowRefusesAGrantWithoutAVerifiedDestination(t *testing.T) {
	w := NewApprovalWindow(newClock().now)
	for name, g := range map[string]Grant{
		"no destination": {SessionID: "s1", IdentityID: "k1"},
		"no session":     {IdentityID: "k1", Destination: "fp"},
		"no key":         {SessionID: "s1", Destination: "fp"},
	} {
		if err := w.Allow(g, time.Minute); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("%s: err = %v; a window needs every field", name, err)
		}
	}
}

func TestApprovalWindowRefusesOutOfRangeDurations(t *testing.T) {
	w := NewApprovalWindow(newClock().now)
	for _, ttl := range []time.Duration{0, -time.Second, MaxWindowMinutes*time.Minute + time.Nanosecond} {
		if err := w.Allow(grant, ttl); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("ttl %s accepted", ttl)
		}
	}
	if err := w.Allow(grant, MaxWindowMinutes*time.Minute); err != nil {
		t.Errorf("the maximum window must be accepted: %v", err)
	}
}

func TestApprovalWindowForgetsAClosedSession(t *testing.T) {
	w := NewApprovalWindow(newClock().now)
	other := Grant{SessionID: "s2", IdentityID: "k1", Destination: "fp-b"}
	for _, g := range []Grant{grant, other} {
		if err := w.Allow(g, time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	w.ForgetSession("s1")
	if w.Allows(grant) {
		t.Error("closing a session must end its windows")
	}
	if !w.Allows(other) {
		t.Error("closing one session must not end another's windows")
	}
}

func TestApprovalWindowStaysBounded(t *testing.T) {
	clock := newClock()
	w := NewApprovalWindow(clock.now)
	for i := range maxGrants {
		if err := w.Allow(Grant{SessionID: "s", IdentityID: fmt.Sprint(i), Destination: "d"}, time.Minute); err != nil {
			t.Fatalf("grant %d refused: %v", i, err)
		}
	}
	if err := w.Allow(Grant{SessionID: "s", IdentityID: "extra", Destination: "d"}, time.Minute); !errors.Is(err, ErrInvalidConfig) {
		t.Fatal("the grant table must stay bounded")
	}
	clock.advance(time.Minute)
	if err := w.Allow(Grant{SessionID: "s", IdentityID: "extra", Destination: "d"}, time.Minute); err != nil {
		t.Fatalf("expired grants must be pruned to make room: %v", err)
	}
}

// --- limiter ---

func TestRequestLimiterAllowsExactlyMaxPerInterval(t *testing.T) {
	clock := newClock()
	l := NewRequestLimiter(3, 10*time.Second, clock.now)
	for i := range 3 {
		if err := l.Allow(); err != nil {
			t.Fatalf("request %d refused: %v", i+1, err)
		}
	}
	if err := l.Allow(); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("request 4 in the interval: err = %v, want ErrRateLimited", err)
	}
	clock.advance(10*time.Second - time.Nanosecond)
	if err := l.Allow(); !errors.Is(err, ErrRateLimited) {
		t.Fatal("the interval has not passed yet")
	}
	clock.advance(time.Nanosecond)
	if err := l.Allow(); err != nil {
		t.Fatalf("a request exactly one interval later must be allowed: %v", err)
	}
}

func TestRequestLimiterDoesNotCountRefusals(t *testing.T) {
	clock := newClock()
	l := NewRequestLimiter(1, 10*time.Second, clock.now)
	if err := l.Allow(); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		clock.advance(time.Second)
		_ = l.Allow()
	}
	clock.advance(5 * time.Second)
	if err := l.Allow(); err != nil {
		t.Fatalf("refused requests must not extend the penalty: %v", err)
	}
}

// --- config ---

func TestConfigValidateAcceptsDraftsAndRefusesMalformed(t *testing.T) {
	var nilCfg *Config
	if err := nilCfg.Validate(); err != nil {
		t.Errorf("no configuration is valid: %v", err)
	}
	ok := []Config{
		{},
		{Enabled: true},
		{Enabled: true, IdentityIDs: []string{"a", "b"}, Confirm: ConfirmEach},
		{Enabled: true, IdentityIDs: []string{"a"}, Confirm: ConfirmSessionWindow, WindowMinutes: MaxWindowMinutes},
	}
	for i, c := range ok {
		if err := c.Validate(); err != nil {
			t.Errorf("case %d: %v", i, err)
		}
	}
	tooMany := make([]string, MaxForwardedKeys+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprint(i)
	}
	bad := map[string]Config{
		"unknown policy":  {Confirm: "always"},
		"negative window": {WindowMinutes: -1},
		"window too long": {WindowMinutes: MaxWindowMinutes + 1},
		"empty id":        {IdentityIDs: []string{""}},
		"duplicate id":    {IdentityIDs: []string{"a", "a"}},
		"too many keys":   {IdentityIDs: tooMany},
	}
	for name, c := range bad {
		if err := c.Validate(); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("%s: err = %v, want ErrInvalidConfig", name, err)
		}
	}
	if err := (&Config{IdentityIDs: tooMany[:MaxForwardedKeys]}).Validate(); err != nil {
		t.Errorf("exactly MaxForwardedKeys must be accepted: %v", err)
	}
}

func TestConfigValidateForConnectNeedsAKeyWhenEnabled(t *testing.T) {
	if err := (&Config{Enabled: true}).ValidateForConnect(); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("enabled with no key: err = %v", err)
	}
	if err := (&Config{Enabled: false}).ValidateForConnect(); err != nil {
		t.Errorf("disabled with no key is fine: %v", err)
	}
	if err := (&Config{Enabled: true, IdentityIDs: []string{"a"}}).ValidateForConnect(); err != nil {
		t.Errorf("enabled with a key is fine: %v", err)
	}
	if err := (&Config{Enabled: true, IdentityIDs: []string{"a"}, Confirm: "x"}).ValidateForConnect(); !errors.Is(err, ErrInvalidConfig) {
		t.Error("the strict check must include the structural one")
	}
	var nilCfg *Config
	if err := nilCfg.ValidateForConnect(); err != nil {
		t.Errorf("no configuration is valid: %v", err)
	}
}

func TestConfigListsOnlySelectedKeysOfAnActiveConfig(t *testing.T) {
	c := &Config{Enabled: true, IdentityIDs: []string{"a", "b"}}
	if !c.Lists("a") || !c.Lists("b") || c.Lists("c") {
		t.Error("Lists must report exactly the selected keys")
	}
	c.Enabled = false
	if c.Lists("a") || c.Active() {
		t.Error("a disabled configuration forwards nothing")
	}
	var nilCfg *Config
	if nilCfg.Lists("a") || nilCfg.Active() {
		t.Error("no configuration forwards nothing")
	}
	if (&Config{Enabled: true}).Active() {
		t.Error("enabled with no keys is not active")
	}
}

func TestConfigWindowIsOfferedOnlyBySessionWindowPolicy(t *testing.T) {
	cases := []struct {
		cfg  *Config
		want time.Duration
	}{
		{nil, 0},
		{&Config{Confirm: ConfirmEach, WindowMinutes: 30}, 0},
		{&Config{}, 0},
		{&Config{Confirm: ConfirmSessionWindow}, DefaultWindowMinutes * time.Minute},
		{&Config{Confirm: ConfirmSessionWindow, WindowMinutes: 5}, 5 * time.Minute},
	}
	for i, c := range cases {
		if got := c.cfg.Window(); got != c.want {
			t.Errorf("case %d: Window() = %s, want %s", i, got, c.want)
		}
	}
}
