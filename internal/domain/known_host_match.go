package domain

import (
	"crypto/hmac"
	// #nosec G505 -- the known_hosts hashed-host format is defined as HMAC-SHA1; see
	// hashedKnownHostMatches for why nothing here depends on SHA-1 being collision resistant.
	"crypto/sha1"
	"encoding/base64"
	"strings"
)

// KnownHostMatches reports whether a known_hosts host field designates host.
//
// It lives in the domain, not beside the known_hosts repository, because two places must agree on
// it exactly: the repository deciding which stored lines vouch for a host, and sync deciding whether
// a line arriving from another device would vouch for a host this device already trusts. A sync
// check looser than the repository's lookup is a way to plant a second key for a known host.
//
// It handles the hashed form OpenSSH writes when HashKnownHosts is on, which a plain string
// comparison can never match: `|1|<base64 salt>|<base64 HMAC-SHA1(salt, host)>`. Before this, a
// known_hosts file imported from OpenSSH with hashing enabled contributed nothing - every entry
// silently failed to match, the host came back as unknown, and the user was walked through
// trust-on-first-use again for a host they had already verified. Re-establishing trust from
// scratch is exactly the moment an attacker wants.
func KnownHostMatches(lineHost, host string) bool {
	if IsHashedKnownHost(lineHost) {
		return hashedKnownHostMatches(lineHost, strings.TrimSpace(host))
	}
	return strings.TrimSpace(lineHost) == strings.TrimSpace(host)
}

// IsHashedKnownHost reports whether a known_hosts host field is in OpenSSH's salted, hashed form,
// which names a host without revealing it.
func IsHashedKnownHost(lineHost string) bool { return strings.HasPrefix(lineHost, "|1|") }

func hashedKnownHostMatches(entry, host string) bool {
	parts := strings.Split(entry, "|")
	// A hashed entry splits as ["", "1", salt, hash]: the leading empty field is the text before
	// the first separator.
	if len(parts) != 4 || parts[1] != "1" {
		return false
	}
	salt, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	// SHA-1 is not a choice here: HMAC-SHA1 is what the known_hosts hashed-host format specifies,
	// and this reads files OpenSSH wrote. Nothing is being authenticated by this digest - a match
	// only selects which lines to compare the host key against, and the key comparison is what
	// decides trust.
	// #nosec G401 -- required by the OpenSSH known_hosts wire format, not a security decision here
	mac := hmac.New(sha1.New, salt)
	mac.Write([]byte(host))
	return hmac.Equal(mac.Sum(nil), want)
}
