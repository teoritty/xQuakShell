package ssh

import (
	"crypto/rsa"
	"errors"
	"fmt"

	gossh "golang.org/x/crypto/ssh"
)

// ErrWeakHostKey indicates a server presented a host key whose parameters put it below the
// minimum this client accepts. It is not a trust decision - the key never reaches known_hosts -
// so it is reported separately from ErrUnknownHost and ErrHostKeyMismatch.
var ErrWeakHostKey = errors.New("server host key is too weak")

// The SSH algorithm policy for every connection this application makes.
//
// It is written down rather than inherited from golang.org/x/crypto/ssh's defaults, and the
// difference is not cosmetic. A default is whatever the library decided in the version that
// happens to be vendored: it moves when the dependency is bumped, in either direction, with no
// review and nothing that fails when it does. What a client will negotiate is a security decision
// this application owns, so it lives here, next to the reasoning, where a change to it shows up in
// a diff.
//
// Everything below is expressed with the library's named constants, so an algorithm that is
// renamed or withdrawn upstream breaks the build instead of silently dropping out of the list.
var (
	// hostKeyAlgorithms are the host key algorithms offered during negotiation.
	//
	// ssh-rsa is absent: it means RSA with a SHA-1 signature, which is why OpenSSH turned it off
	// by default in 8.8. rsa-sha2-256 and rsa-sha2-512 are the same RSA host keys under a sound
	// hash, so a server with only an RSA key still connects. InsecureKeyAlgoDSA is absent for the
	// obvious reason and because its name says so.
	hostKeyAlgorithms = []string{
		gossh.KeyAlgoED25519,
		gossh.KeyAlgoSKED25519,
		gossh.KeyAlgoECDSA256,
		gossh.KeyAlgoECDSA384,
		gossh.KeyAlgoECDSA521,
		gossh.KeyAlgoSKECDSA256,
		gossh.KeyAlgoRSASHA512,
		gossh.KeyAlgoRSASHA256,
		gossh.CertAlgoED25519v01,
		gossh.CertAlgoECDSA256v01,
		gossh.CertAlgoECDSA384v01,
		gossh.CertAlgoECDSA521v01,
		gossh.CertAlgoRSASHA512v01,
		gossh.CertAlgoRSASHA256v01,
	}

	// keyExchanges excludes every SHA-1 exchange and the 1024-bit group. Curve25519 is first
	// because it is the one to use; the NIST curves and the larger DH groups follow for servers
	// that do not offer it.
	keyExchanges = []string{
		gossh.KeyExchangeCurve25519,
		gossh.KeyExchangeECDHP256,
		gossh.KeyExchangeECDHP384,
		gossh.KeyExchangeECDHP521,
		gossh.KeyExchangeDH16SHA512,
		gossh.KeyExchangeDHGEXSHA256,
		gossh.KeyExchangeDH14SHA256,
	}

	// ciphers are AEAD first, then CTR. No CBC: SSH's CBC modes carry the Encrypt-and-MAC
	// construction that made them attackable, and nothing current requires them. No RC4.
	ciphers = []string{
		gossh.CipherChaCha20Poly1305,
		gossh.CipherAES256GCM,
		gossh.CipherAES128GCM,
		gossh.CipherAES256CTR,
		gossh.CipherAES192CTR,
		gossh.CipherAES128CTR,
	}

	// macs prefer encrypt-then-MAC. These apply only to the CTR ciphers above - an AEAD cipher
	// carries its own authentication and negotiates no separate MAC. HMACSHA1 is absent.
	macs = []string{
		gossh.HMACSHA256ETM,
		gossh.HMACSHA512ETM,
		gossh.HMACSHA256,
		gossh.HMACSHA512,
	}
)

// applyCryptoPolicy pins the negotiable algorithms on an outgoing client config.
func applyCryptoPolicy(cfg *gossh.ClientConfig) {
	cfg.HostKeyAlgorithms = hostKeyAlgorithms
	cfg.KeyExchanges = keyExchanges
	cfg.Ciphers = ciphers
	cfg.MACs = macs
}

// MinRSAHostKeyBits is the smallest RSA host key this client will talk to.
//
// 2048 is the floor rather than a preference: 1024-bit RSA is within reach of a well-resourced
// attacker and has been withdrawn from every current recommendation. It is deliberately not
// higher, because a host key is not rotated casually and refusing 2048 would strand servers that
// are not actually broken.
const MinRSAHostKeyBits = 2048

// checkHostKeyStrength rejects a host key whose parameters make it worthless as an identity.
//
// This is separate from algorithm negotiation and cannot be folded into it. rsa-sha2-256 is a
// sound signature algorithm and says nothing about the size of the modulus signing with it, so a
// 1024-bit host key arrives over a policy that correctly refused ssh-rsa.
func checkHostKeyStrength(key gossh.PublicKey) error {
	if key == nil {
		return ErrWeakHostKey
	}
	cryptoKey, ok := key.(gossh.CryptoPublicKey)
	if !ok {
		// A key type this library models without exposing the underlying crypto key. There is
		// nothing to measure, and the algorithm lists above already decide which types appear.
		return nil
	}
	rsaKey, ok := cryptoKey.CryptoPublicKey().(*rsa.PublicKey)
	if !ok {
		// Ed25519 and the NIST curves have one size each, fixed by the algorithm name that was
		// negotiated. There is no weak variant to catch.
		return nil
	}
	if rsaKey.N.BitLen() < MinRSAHostKeyBits {
		return fmt.Errorf("%w: %d-bit RSA host key, minimum is %d",
			ErrWeakHostKey, rsaKey.N.BitLen(), MinRSAHostKeyBits)
	}
	return nil
}
