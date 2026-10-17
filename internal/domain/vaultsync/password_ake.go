package vaultsync

import "errors"

// Errors of the password-authenticated key exchange. They are local errors, not integrity errors:
// a wrong password is the ordinary case, not evidence of tampering.
var (
	// ErrAuthFailed covers a wrong password, an unknown account and a tampered exchange alike. One
	// error for all three is what keeps a login from telling an attacker which accounts exist.
	ErrAuthFailed = errors.New("password authentication failed")
	// ErrMalformedMessage is a protocol message that does not decode.
	ErrMalformedMessage = errors.New("malformed password authentication message")
)

// PasswordAKEClient is the client half of OPAQUE (RFC 9807) for one exchange. Messages are opaque
// bytes; the adapter behind this port fixes the suite, so a caller cannot weaken it.
type PasswordAKEClient interface {
	RegistrationStart(password []byte) (request []byte, err error)
	RegistrationFinish(response []byte, account ID) (record, exportKey []byte, err error)
	LoginStart(password []byte) (ke1 []byte, err error)
	LoginFinish(ke2 []byte, account ID) (ke3, sessionKey, exportKey []byte, err error)
}

// PasswordAKEServer is the server half. Its key material and OPRF seed come from the key file kept
// outside the database (spec section 9.4), which is why a stolen database alone does not allow
// offline guessing.
type PasswordAKEServer interface {
	RegistrationResponse(request []byte, account ID) (response []byte, err error)
	ValidateRecord(record []byte) error
	LoginStart(ke1, record []byte, account ID) (ke2, expectedClientMAC, sessionKey []byte, err error)
	LoginStartUnknown(ke1 []byte, account ID) (ke2 []byte, err error)
	LoginFinish(ke3, expectedClientMAC []byte) error
}
