package agentforward

import "encoding/binary"

// wire builds SSH wire-format byte strings for tests. It encodes; it does not parse, so a test that
// uses it is not re-implementing the parser under test.
type wire []byte

func (w wire) str(s []byte) wire {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(s)))
	return append(append(w, n[:]...), s...)
}

func (w wire) text(s string) wire { return w.str([]byte(s)) }

func (w wire) byte1(b byte) wire { return append(w, b) }

func (w wire) bool1(v bool) wire {
	if v {
		return append(w, 1)
	}
	return append(w, 0)
}

// keyBlob is a structurally valid public key blob: the algorithm name followed by opaque bytes.
// The parser only ever reads the leading name, which is all a key type check needs.
func keyBlob(format string) []byte {
	return wire{}.text(format).str([]byte("opaque-key-material")).bytes()
}

func (w wire) bytes() []byte { return []byte(w) }

var testSessionID = []byte("0123456789abcdef0123456789abcdef")

// loginRequest is the data an OpenSSH client asks the agent to sign for a publickey login.
func loginRequest(user, algorithm string, blob []byte) []byte {
	return wire{}.str(testSessionID).byte1(msgUserAuthRequest).
		text(user).text("ssh-connection").text("publickey").bool1(true).
		text(algorithm).str(blob).bytes()
}

// hostboundRequest is the publickey-hostbound-v00@openssh.com variant OpenSSH 8.9+ sends when the
// server advertises it: the same fields followed by the server's host key.
func hostboundRequest(user, algorithm string, blob, hostKey []byte) []byte {
	return wire{}.str(testSessionID).byte1(msgUserAuthRequest).
		text(user).text("ssh-connection").text(methodHostbound).bool1(true).
		text(algorithm).str(blob).str(hostKey).bytes()
}
