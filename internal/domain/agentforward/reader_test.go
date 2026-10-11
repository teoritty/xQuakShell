package agentforward

import (
	"bytes"
	"errors"
	"testing"
)

// The reader's bounds are where an off-by-one reads past a buffer or refuses a valid message, so
// each one is pinned at the exact length where the answer changes.

func TestReaderReadsALastByteExactly(t *testing.T) {
	r := reader{buf: []byte{1}}
	if !r.bool1() || !r.done() {
		t.Fatal("a one-byte buffer holds exactly one boolean")
	}
	empty := reader{}
	_ = empty.byte1()
	if empty.done() {
		t.Fatal("reading a byte from an empty buffer must fail")
	}
}

func TestReaderReadsAnEmptyStringAtTheEnd(t *testing.T) {
	r := reader{buf: []byte{0, 0, 0, 0}}
	if s := r.str(); len(s) != 0 || !r.done() {
		t.Fatalf("four zero bytes are one empty string: %q, done=%v", s, r.done())
	}
	short := reader{buf: []byte{0, 0, 0}}
	_ = short.str()
	if short.done() {
		t.Fatal("three bytes cannot hold a length prefix")
	}
}

func TestReaderStringFillsTheBufferExactly(t *testing.T) {
	r := reader{buf: []byte{0, 0, 0, 2, 'h', 'i'}}
	if s := r.str(); string(s) != "hi" || !r.done() {
		t.Fatalf("a length equal to what remains must be read whole: %q", s)
	}
	over := reader{buf: []byte{0, 0, 0, 3, 'h', 'i'}}
	if s := over.str(); s != nil || over.done() {
		t.Fatal("a length one past what remains must fail")
	}
}

func TestReaderStaysFailedAfterAFailure(t *testing.T) {
	r := reader{buf: []byte{0, 0, 0, 9}}
	_ = r.str()
	r.buf = []byte{1}
	if r.byte1() != 0 || r.done() {
		t.Fatal("a reader that failed once must not read again")
	}
}

func TestParseUserAuthRequestBoundsKeyBlobs(t *testing.T) {
	maxBlob := bytes.Repeat([]byte{1}, maxKeyBlobLen)
	if _, err := ParseUserAuthRequest(loginRequest("u", "ssh-ed25519", maxBlob)); err != nil {
		t.Errorf("a key blob of exactly the limit must be accepted: %v", err)
	}
	long := append(maxBlob, 1)
	if _, err := ParseUserAuthRequest(loginRequest("u", "ssh-ed25519", long)); !errors.Is(err, ErrSignDataRejected) {
		t.Error("a key blob one past the limit must be refused")
	}
	if _, err := ParseUserAuthRequest(hostboundRequest("u", "ssh-ed25519", keyBlob("ssh-ed25519"), maxBlob)); err != nil {
		t.Errorf("a host key of exactly the limit must be accepted: %v", err)
	}
	if _, err := ParseUserAuthRequest(hostboundRequest("u", "ssh-ed25519", keyBlob("ssh-ed25519"), long)); !errors.Is(err, ErrSignDataRejected) {
		t.Error("a host key one past the limit must be refused")
	}
}

func TestBindingsAcceptTheLongestSessionID(t *testing.T) {
	var b Bindings
	nb := Binding{SessionID: bytes.Repeat([]byte{1}, MaxSessionIDLen), HostKey: []byte("k"), HostKeyFingerprint: "f"}
	if err := b.Add(nb); err != nil {
		t.Fatalf("a SHA-512 session id must be bindable: %v", err)
	}
}
