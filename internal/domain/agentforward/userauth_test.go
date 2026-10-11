package agentforward

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestParseUserAuthRequestAcceptsAPublickeyLogin(t *testing.T) {
	blob := keyBlob("ssh-ed25519")
	req, err := ParseUserAuthRequest(loginRequest("deploy", "ssh-ed25519", blob))
	if err != nil {
		t.Fatalf("a well-formed publickey login was refused: %v", err)
	}
	if req.User != "deploy" || req.Algorithm != "ssh-ed25519" {
		t.Errorf("parsed user=%q algorithm=%q, want deploy / ssh-ed25519", req.User, req.Algorithm)
	}
	if !bytes.Equal(req.SessionID, testSessionID) || !bytes.Equal(req.KeyBlob, blob) {
		t.Error("session id or key blob not carried through unchanged")
	}
	if req.HostKey != nil {
		t.Error("a plain publickey request must not report a host key")
	}
}

func TestParseUserAuthRequestAcceptsTheHostboundVariant(t *testing.T) {
	hostKey := keyBlob("ssh-ed25519")
	req, err := ParseUserAuthRequest(hostboundRequest("git", "ssh-ed25519", keyBlob("ssh-ed25519"), hostKey))
	if err != nil {
		t.Fatalf("OpenSSH 8.9+ sends publickey-hostbound-v00; refusing it breaks every modern client: %v", err)
	}
	if !bytes.Equal(req.HostKey, hostKey) {
		t.Error("the hostbound variant must report the server host key it names")
	}
}

// Each case is something a hostile server could ask the agent to sign. A refusal is the only
// acceptable outcome, and it must be ErrSignDataRejected so the caller can audit it as such.
func TestParseUserAuthRequestRefusesAnythingButALogin(t *testing.T) {
	blob := keyBlob("ssh-ed25519")
	valid := loginRequest("deploy", "ssh-ed25519", blob)
	cases := map[string][]byte{
		"empty":                   nil,
		"SSHSIG git signature":    append([]byte("SSHSIG"), wire{}.text("git").str(nil).text("sha512").str([]byte("hash")).bytes()...),
		"arbitrary blob":          []byte("sign me, I am definitely a login"),
		"trailing byte":           append(append([]byte{}, valid...), 0),
		"truncated":               valid[:len(valid)-3],
		"wrong message type":      wire{}.str(testSessionID).byte1(51).text("deploy").text("ssh-connection").text("publickey").bool1(true).text("ssh-ed25519").str(blob).bytes(),
		"other service":           wire{}.str(testSessionID).byte1(msgUserAuthRequest).text("deploy").text("ssh-userauth").text("publickey").bool1(true).text("ssh-ed25519").str(blob).bytes(),
		"method none":             wire{}.str(testSessionID).byte1(msgUserAuthRequest).text("deploy").text("ssh-connection").text("none").bytes(),
		"method hostbased":        wire{}.str(testSessionID).byte1(msgUserAuthRequest).text("deploy").text("ssh-connection").text("hostbased").bool1(true).text("ssh-ed25519").str(blob).bytes(),
		"query without signature": wire{}.str(testSessionID).byte1(msgUserAuthRequest).text("deploy").text("ssh-connection").text("publickey").bool1(false).text("ssh-ed25519").str(blob).bytes(),
		"empty session id":        wire{}.str(nil).byte1(msgUserAuthRequest).text("deploy").text("ssh-connection").text("publickey").bool1(true).text("ssh-ed25519").str(blob).bytes(),
		"oversized session id":    wire{}.str(bytes.Repeat([]byte{1}, MaxSessionIDLen+1)).byte1(msgUserAuthRequest).text("deploy").text("ssh-connection").text("publickey").bool1(true).text("ssh-ed25519").str(blob).bytes(),
		"length overflow":         append(wire{}.str(testSessionID).byte1(msgUserAuthRequest).bytes(), 0xff, 0xff, 0xff, 0xff, 'x'),
		"empty user":              loginRequest("", "ssh-ed25519", blob),
		"control char in user":    loginRequest("root\x1b[2J", "ssh-ed25519", blob),
		"newline in user":         loginRequest("root\nadmin", "ssh-ed25519", blob),
		"invalid utf8 user":       loginRequest("ro\xffot", "ssh-ed25519", blob),
		"oversized user":          loginRequest(strings.Repeat("a", MaxUserLen+1), "ssh-ed25519", blob),
		"empty key blob":          loginRequest("deploy", "ssh-ed25519", nil),
		"hostbound without host":  wire{}.str(testSessionID).byte1(msgUserAuthRequest).text("deploy").text("ssh-connection").text(methodHostbound).bool1(true).text("ssh-ed25519").str(blob).bytes(),
		"hostbound empty host":    hostboundRequest("deploy", "ssh-ed25519", blob, nil),
		"bool not 0 or 1":         wire{}.str(testSessionID).byte1(msgUserAuthRequest).text("deploy").text("ssh-connection").text("publickey").byte1(2).text("ssh-ed25519").str(blob).bytes(),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseUserAuthRequest(data)
			if !errors.Is(err, ErrSignDataRejected) {
				t.Fatalf("err = %v, want ErrSignDataRejected; the agent must not sign this", err)
			}
		})
	}
}

func TestParseUserAuthRequestAcceptsTheBoundaryLengths(t *testing.T) {
	blob := keyBlob("ssh-ed25519")
	user := strings.Repeat("a", MaxUserLen)
	if _, err := ParseUserAuthRequest(loginRequest(user, "ssh-ed25519", blob)); err != nil {
		t.Errorf("a user name of exactly MaxUserLen must be accepted: %v", err)
	}
	sid := bytes.Repeat([]byte{7}, MaxSessionIDLen)
	data := wire{}.str(sid).byte1(msgUserAuthRequest).text("u").text("ssh-connection").text("publickey").bool1(true).text("ssh-ed25519").str(blob).bytes()
	if _, err := ParseUserAuthRequest(data); err != nil {
		t.Errorf("a session id of exactly MaxSessionIDLen (SHA-512) must be accepted: %v", err)
	}
}

func TestKeyFormatReadsTheLeadingAlgorithmName(t *testing.T) {
	got, err := KeyFormat(keyBlob("ecdsa-sha2-nistp256"))
	if err != nil || got != "ecdsa-sha2-nistp256" {
		t.Fatalf("KeyFormat = %q, %v; want ecdsa-sha2-nistp256", got, err)
	}
	if _, err := KeyFormat([]byte{0, 0, 0, 9, 'x'}); !errors.Is(err, ErrSignDataRejected) {
		t.Errorf("a truncated key blob must be refused, got %v", err)
	}
}

func FuzzParseUserAuthRequest(f *testing.F) {
	blob := keyBlob("ssh-ed25519")
	f.Add(loginRequest("deploy", "ssh-ed25519", blob))
	f.Add(hostboundRequest("git", "rsa-sha2-512", keyBlob("ssh-rsa"), blob))
	f.Add([]byte("SSHSIG"))
	f.Fuzz(func(t *testing.T, data []byte) {
		req, err := ParseUserAuthRequest(data)
		if err != nil {
			if !errors.Is(err, ErrSignDataRejected) {
				t.Fatalf("every refusal must be ErrSignDataRejected, got %v", err)
			}
			return
		}
		// Anything accepted must satisfy the invariants the policy relies on.
		if len(req.SessionID) == 0 || len(req.SessionID) > MaxSessionIDLen || req.User == "" || len(req.KeyBlob) == 0 {
			t.Fatalf("accepted a request that breaks an invariant: %+v", req)
		}
	})
}
