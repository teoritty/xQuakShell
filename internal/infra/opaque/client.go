package opaque

import (
	lib "github.com/bytemare/opaque"

	"xquakshell/internal/domain/vaultsync"
)

// Client is the client half of one OPAQUE exchange. It is stateful between a Start and its
// Finish and must not be reused across exchanges; the library keeps the blinded password in it.
type Client struct {
	lib *lib.Client
	ksf ksfSettings
}

var _ vaultsync.PasswordAKEClient = (*Client)(nil)

// NewClient returns a suite 1 client bound to the hello transcript hash.
func NewClient(context []byte) (*Client, error) {
	return newClient(context, suite1KSF())
}

func newClient(context []byte, k ksfSettings) (*Client, error) {
	c, err := configuration(context).Client()
	if err != nil {
		return nil, err
	}
	return &Client{lib: c, ksf: k}, nil
}

func (c *Client) options() *lib.ClientOptions {
	return &lib.ClientOptions{KSFParameters: c.ksf.params, KSFLength: c.ksf.length}
}

// RegistrationStart blinds the password for the server's OPRF.
func (c *Client) RegistrationStart(password []byte) ([]byte, error) {
	req, err := c.lib.RegistrationInit(password)
	if err != nil {
		return nil, vaultsync.ErrAuthFailed
	}
	return req.Serialize(), nil
}

// RegistrationFinish produces the record the server stores and the export key that seals the
// account bundle. The account id is the client identity inside the envelope, so the record
// authenticates that account and no other.
func (c *Client) RegistrationFinish(response []byte, account vaultsync.ID) (record, exportKey []byte, err error) {
	defer c.lib.ClearState()
	resp, err := c.lib.Deserialize.RegistrationResponse(response)
	if err != nil {
		return nil, nil, vaultsync.ErrMalformedMessage
	}
	rec, exportKey, err := c.lib.RegistrationFinalize(resp, account[:], nil, c.options())
	if err != nil {
		return nil, nil, vaultsync.ErrAuthFailed
	}
	return rec.Serialize(), exportKey, nil
}

// LoginStart blinds the password and opens the key exchange.
func (c *Client) LoginStart(password []byte) ([]byte, error) {
	ke1, err := c.lib.GenerateKE1(password)
	if err != nil {
		return nil, vaultsync.ErrAuthFailed
	}
	return ke1.Serialize(), nil
}

// LoginFinish authenticates the server, recovers the export key and produces the client's proof.
// Every failure after decoding - wrong password, unknown account, other context, tampering - is the
// same ErrAuthFailed.
func (c *Client) LoginFinish(ke2 []byte, account vaultsync.ID) (ke3, sessionKey, exportKey []byte, err error) {
	defer c.lib.ClearState()
	msg, err := c.lib.Deserialize.KE2(ke2)
	if err != nil {
		return nil, nil, nil, vaultsync.ErrMalformedMessage
	}
	out, sessionKey, exportKey, err := c.lib.GenerateKE3(msg, account[:], nil, c.options())
	if err != nil {
		return nil, nil, nil, vaultsync.ErrAuthFailed
	}
	return out.Serialize(), sessionKey, exportKey, nil
}
