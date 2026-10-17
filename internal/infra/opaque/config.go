// Package opaque adapts github.com/bytemare/opaque (RFC 9807) to the vaultsync password AKE ports,
// fixed to sync suite 1: ristretto255 with SHA-512 for the OPRF and the 3DH key exchange, Argon2id
// at the suite's parameters as the key stretching function, and the hash of the version
// negotiation as the AKE context (ADR-023).
//
// The library is pinned and reviewed; this package is the only code that imports it, so it can be
// replaced without touching the protocol or the stored records.
package opaque

import (
	"bytes"
	"crypto"
	"fmt"

	"github.com/bytemare/ksf"
	lib "github.com/bytemare/opaque"

	"xquakshell/internal/domain/vaultsync"
)

// ksfSettings are the Argon2id parameters, in the order the library takes them: iterations,
// memory in KiB, threads, and the output length.
type ksfSettings struct {
	params []uint64
	length int
}

// suite1KSF reads the stretching cost from the frozen suite. The library's own default is 64 MiB;
// passing these explicitly on every call is what keeps that default from ever applying.
func suite1KSF() ksfSettings {
	p, err := vaultsync.LookupSuite(vaultsync.Suite1, vaultsync.MinAcceptedSuite)
	if err != nil {
		// Suite 1 is compiled in; failing to find it is a broken build, and refusing every login
		// is the safe response rather than guessing parameters.
		return ksfSettings{}
	}
	a := p.Argon2
	return ksfSettings{
		params: []uint64{uint64(a.Iterations), uint64(a.MemoryKiB), uint64(a.Threads)},
		length: int(a.KeyLen),
	}
}

// configuration builds the library configuration for suite 1 with the given AKE context. The
// context is the hello transcript hash, so a client and server that saw different negotiations
// derive different keys and the login fails.
func configuration(context []byte) *lib.Configuration {
	return &lib.Configuration{
		OPRF:    lib.RistrettoSha512,
		AKE:     lib.RistrettoSha512,
		KSF:     ksf.Argon2id,
		KDF:     crypto.SHA512,
		MAC:     crypto.SHA512,
		Hash:    crypto.SHA512,
		Context: bytes.Clone(context),
	}
}

// GenerateServerKeyMaterial creates the server's long-term AKE key pair and OPRF seed, encoded for
// the key file. It is generated once at server init and never stored in the database.
func GenerateServerKeyMaterial() ([]byte, error) {
	conf := configuration(nil)
	sk, pk := conf.KeyGen()
	// The identity is the public key itself: that is what a client passing no server identity
	// binds to, and the key file encoding has no room for an empty field.
	skm := &lib.ServerKeyMaterial{
		PrivateKey:     sk,
		PublicKeyBytes: pk.Encode(),
		OPRFGlobalSeed: conf.GenerateOPRFSeed(),
		Identity:       pk.Encode(),
	}
	defer skm.Flush()
	encoded := skm.Encode()
	if len(encoded) == 0 {
		return nil, fmt.Errorf("opaque: encode server key material")
	}
	return encoded, nil
}
