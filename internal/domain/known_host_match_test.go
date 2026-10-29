package domain_test

import (
	"crypto/hmac"
	"crypto/sha1" // #nosec G505 -- builds OpenSSH's hashed known_hosts form, which is HMAC-SHA1
	"encoding/base64"
	"testing"

	"xquakshell/internal/domain"
)

func hashed(host string) string {
	salt := []byte("0123456789abcdefghij")
	mac := hmac.New(sha1.New, salt)
	mac.Write([]byte(host))
	return "|1|" + base64.StdEncoding.EncodeToString(salt) + "|" + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func TestKnownHostMatches(t *testing.T) {
	cases := []struct {
		line, host string
		want       bool
	}{
		{"web.example", "web.example", true},
		{" web.example ", "web.example", true},
		{"web.example", "db.example", false},
		{"[web.example]:2222", "[web.example]:2222", true},
		{"[web.example]:2222", "web.example", false},
		{hashed("web.example"), "web.example", true},
		{hashed("web.example"), "db.example", false},
		{"|1|not base64|x", "web.example", false},
		{"|1|" + base64.StdEncoding.EncodeToString([]byte("salt")), "web.example", false},
		{"|2|" + hashed("web.example")[3:], "web.example", false},
	}
	for _, c := range cases {
		if got := domain.KnownHostMatches(c.line, c.host); got != c.want {
			t.Errorf("KnownHostMatches(%q, %q) = %v, want %v", c.line, c.host, got, c.want)
		}
	}
}
