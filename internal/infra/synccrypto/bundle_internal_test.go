package synccrypto

import (
	"bytes"
	"errors"
	"testing"

	"xquakshell/internal/domain/vaultsync"
)

func TestDecodeBundleRefusesOtherLayouts(t *testing.T) {
	seed := bytes.Repeat([]byte{1}, seedSize)
	build := func(label string, fields ...[]byte) []byte {
		c := vaultsync.NewCanon(label)
		for _, f := range fields {
			c.Bytes(f)
		}
		b, _ := c.Encoded()
		return b
	}
	cases := map[string][]byte{
		"short seed":    build(vaultsync.LabelBundle, seed[:31], seed),
		"other label":   build("xqs-sync/v1/other", seed, seed),
		"extra field":   build(vaultsync.LabelBundle, seed, seed, seed),
		"missing field": build(vaultsync.LabelBundle, seed),
		"truncated":     build(vaultsync.LabelBundle, seed, seed)[:50],
		"trailing byte": append(build(vaultsync.LabelBundle, seed, seed), 0),
	}
	for name, plain := range cases {
		if _, err := decodeBundle(plain); !errors.Is(err, ErrOpen) {
			t.Errorf("%s: err = %v, want ErrOpen; a bundle of another layout must not be half-read", name, err)
		}
	}
	if _, err := decodeBundle(build(vaultsync.LabelBundle, seed, seed)); err != nil {
		t.Fatalf("the canonical layout failed to decode: %v", err)
	}
}
