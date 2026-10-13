package opaque

import (
	lib "github.com/bytemare/opaque"

	"xquakshell/internal/domain/vaultsync"
)

// Server is the server half of OPAQUE. The library server is safe for concurrent exchanges; the
// per-login state the server needs (the expected client MAC) is returned to the caller to hold.
type Server struct {
	conf *lib.Configuration
	lib  *lib.Server
}

var _ vaultsync.PasswordAKEServer = (*Server)(nil)

// NewServer returns a suite 1 server bound to the hello transcript hash, with the key material
// from the server key file.
func NewServer(context, keyMaterial []byte) (*Server, error) {
	return newServer(context, keyMaterial)
}

func newServer(context, keyMaterial []byte) (*Server, error) {
	conf := configuration(context)
	skm, err := conf.DecodeServerKeyMaterial(keyMaterial)
	if err != nil {
		return nil, vaultsync.ErrMalformedMessage
	}
	s, err := conf.Server()
	if err != nil {
		return nil, err
	}
	if err := s.SetKeyMaterial(skm); err != nil {
		return nil, vaultsync.ErrMalformedMessage
	}
	return &Server{conf: conf, lib: s}, nil
}

// RegistrationResponse evaluates the client's blinded password under the per-account OPRF key
// derived from the seed, so the server never sees the password.
func (s *Server) RegistrationResponse(request []byte, account vaultsync.ID) ([]byte, error) {
	req, err := s.lib.Deserialize.RegistrationRequest(request)
	if err != nil {
		return nil, vaultsync.ErrMalformedMessage
	}
	resp, err := s.lib.RegistrationResponse(req, account[:], nil)
	if err != nil {
		return nil, vaultsync.ErrMalformedMessage
	}
	return resp.Serialize(), nil
}

// ValidateRecord checks that a record the client uploads decodes, before it is stored.
func (s *Server) ValidateRecord(record []byte) error {
	if _, err := s.lib.Deserialize.RegistrationRecord(record); err != nil {
		return vaultsync.ErrMalformedMessage
	}
	return nil
}

// LoginStart answers a login for a known account. The expected client MAC and the session key are
// returned for the caller to keep until LoginFinish.
func (s *Server) LoginStart(ke1, record []byte, account vaultsync.ID) (ke2, expectedClientMAC, sessionKey []byte, err error) {
	msg, err := s.lib.Deserialize.KE1(ke1)
	if err != nil {
		return nil, nil, nil, vaultsync.ErrMalformedMessage
	}
	rec, err := s.lib.Deserialize.RegistrationRecord(record)
	if err != nil {
		return nil, nil, nil, vaultsync.ErrMalformedMessage
	}
	client := &lib.ClientRecord{RegistrationRecord: rec, CredentialIdentifier: account[:], ClientIdentity: account[:]}
	out, output, err := s.lib.GenerateKE2(msg, client)
	if err != nil {
		return nil, nil, nil, vaultsync.ErrAuthFailed
	}
	return out.Serialize(), output.ClientMAC, output.SessionSecret, nil
}

// LoginStartUnknown answers a login for an account that does not exist with a fake record, so the
// response has the shape of a real one and the client fails exactly as it would on a wrong
// password. Without it a login would be an account-existence oracle.
func (s *Server) LoginStartUnknown(ke1 []byte, account vaultsync.ID) ([]byte, error) {
	msg, err := s.lib.Deserialize.KE1(ke1)
	if err != nil {
		return nil, vaultsync.ErrMalformedMessage
	}
	fake, err := s.conf.GetFakeRecord(account[:])
	if err != nil {
		return nil, vaultsync.ErrAuthFailed
	}
	fake.ClientIdentity = account[:]
	out, _, err := s.lib.GenerateKE2(msg, fake)
	if err != nil {
		return nil, vaultsync.ErrAuthFailed
	}
	return out.Serialize(), nil
}

// LoginFinish verifies the client's proof against the MAC LoginStart returned.
func (s *Server) LoginFinish(ke3, expectedClientMAC []byte) error {
	msg, err := s.lib.Deserialize.KE3(ke3)
	if err != nil {
		return vaultsync.ErrMalformedMessage
	}
	if err := s.lib.LoginFinish(msg, expectedClientMAC); err != nil {
		return vaultsync.ErrAuthFailed
	}
	return nil
}
