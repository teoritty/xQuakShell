package agentforward

import "fmt"

// Flags are the SSH_AGENT_SIGN_REQUEST flags of PROTOCOL.agent §5.3.
type Flags uint32

const (
	flagReserved Flags = 1 << iota
	// FlagRSASHA256 asks for an rsa-sha2-256 signature.
	FlagRSASHA256
	// FlagRSASHA512 asks for an rsa-sha2-512 signature.
	FlagRSASHA512
)

const keyFormatRSA = "ssh-rsa"

// signsAsItself lists the key formats whose signature algorithm is their own name. DSA, security
// keys and certificates are absent: the vault can hold none of them, and a format we cannot hold is
// a format no legitimate request will name.
var signsAsItself = map[string]bool{
	"ssh-ed25519":         true,
	"ecdsa-sha2-nistp256": true,
	"ecdsa-sha2-nistp384": true,
	"ecdsa-sha2-nistp521": true,
}

// SigningAlgorithm returns the algorithm to sign with when the key, the algorithm the login data
// names and the request's flags all agree, and refuses otherwise.
//
// Agreement matters because the signature has to be the one the login data claims. A request
// whose flags ask for SHA-512 while its data says SHA-256 is either a broken client or a probe,
// and the safe answer to both is no. RSA with SHA-1 ("ssh-rsa" as an algorithm) is refused
// outright, in step with the crypto policy the client itself negotiates with.
func SigningAlgorithm(keyFormat, algorithm string, flags Flags) (string, error) {
	rsaFlags := flags & (FlagRSASHA256 | FlagRSASHA512)
	if flags&^(FlagRSASHA256|FlagRSASHA512) != 0 {
		return "", refuseAlgorithm("unsupported flags %#x", uint32(flags))
	}
	if keyFormat == keyFormatRSA {
		switch {
		case algorithm == "rsa-sha2-256" && rsaFlags == FlagRSASHA256:
			return algorithm, nil
		case algorithm == "rsa-sha2-512" && rsaFlags == FlagRSASHA512:
			return algorithm, nil
		default:
			return "", refuseAlgorithm("rsa algorithm %q with flags %#x", algorithm, uint32(flags))
		}
	}
	if !signsAsItself[keyFormat] || algorithm != keyFormat || rsaFlags != 0 {
		return "", refuseAlgorithm("key %q, algorithm %q, flags %#x", keyFormat, algorithm, uint32(flags))
	}
	return algorithm, nil
}

func refuseAlgorithm(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrAlgorithmRefused}, args...)...)
}
