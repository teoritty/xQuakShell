package plugin

import (
	"crypto/ed25519"
	"fmt"
)

// InstallTrustPolicy controls signature verification during plugin install.
type InstallTrustPolicy struct {
	TrustedKeys   []ed25519.PublicKey
	RequireSigned bool
}

// EvaluateInstallTrust checks manifest signature against the trust policy.
func EvaluateInstallTrust(m Manifest, policy InstallTrustPolicy) (InstallTrustResult, error) {
	res := InstallTrustResult{
		Signed:          m.Signature != "",
		ChecksumPresent: false,
	}
	if m.Capabilities.Session != nil && m.Capabilities.Session.AllowMultiSession {
		res.MultiSessionWarning = true
	}
	if m.Signature == "" {
		if policy.RequireSigned {
			return res, fmt.Errorf("%w: signed plugin required", ErrInvalidManifest)
		}
		res.UnsignedWarning = true
		return res, nil
	}

	ok, err := VerifyManifestSignature(m, policy.TrustedKeys)
	if err != nil {
		return res, err
	}
	res.SignatureVerified = ok
	if !ok {
		if policy.RequireSigned {
			return res, fmt.Errorf("%w: signature not trusted", ErrInvalidManifest)
		}
		res.UntrustedSignatureWarning = true
	}
	return res, nil
}

// InstallTrustResult summarizes install-time trust checks.
type InstallTrustResult struct {
	Signed                    bool
	SignatureVerified         bool
	UnsignedWarning           bool
	UntrustedSignatureWarning bool
	MultiSessionWarning       bool
	ChecksumPresent           bool
}
