package vault

import (
	"fmt"

	"filippo.io/age"

	"xquakshell/internal/domain"
)

// Session is an opened vault file: the vault key that decrypts its payload, plus the wraps that
// have to survive the next write.
//
// Carrying the wraps is not bookkeeping, it is the correctness of the whole feature. A write
// re-encrypts the payload and rewrites the file; if the wrap belonging to the credential that was
// not used to open the vault were not carried through, the first save after a password unlock would
// silently revoke the recovery key.
//
// It holds no credential. Once the vault is open the master password has done its job, and keeping
// it for the life of the process - which the old format required, because every flush re-derived
// from it - was a secret sitting in memory for no benefit.
type Session struct {
	dir    string
	key    *age.X25519Identity
	env    envelope
	legacy bool
}

// CreateSession mints a vault key for a new vault and wraps it under the master password.
//
// Nothing is written here. The caller decides when the first payload exists, so a failure between
// this and the first Save leaves no half-made vault on disk.
func CreateSession(dir, masterPassword string) (*Session, error) {
	key, err := newVaultKey()
	if err != nil {
		return nil, err
	}

	s := &Session{dir: dir, key: key, env: envelope{Envelope: CurrentEnvelopeVersion}}
	if err := s.SetPasswordWrap(masterPassword); err != nil {
		return nil, err
	}
	return s, nil
}

// Open reads the vault and opens it with either credential, reporting which one worked.
func Open(dir, credential string) (*Session, *domain.VaultData, domain.UnlockMethod, error) {
	raw, err := readVaultBytes(dir)
	if err != nil {
		return nil, nil, domain.UnlockByPassword, err
	}

	if IsLegacyAgeFile(raw) {
		data, err := DecryptLegacy(raw, credential)
		if err != nil {
			return nil, nil, domain.UnlockByPassword, err
		}
		// The vault key is minted by ConvertLegacy, not here: a read that merely inspects the vault
		// must not be the thing that rewrites its format.
		return &Session{dir: dir, legacy: true}, data, domain.UnlockByPassword, nil
	}

	env, err := parseEnvelope(raw)
	if err != nil {
		return nil, nil, domain.UnlockByPassword, err
	}
	key, method, err := openEnvelope(env, credential)
	if err != nil {
		return nil, nil, domain.UnlockByPassword, err
	}
	data, err := decryptPayload(env.Payload, key)
	if err != nil {
		return nil, nil, domain.UnlockByPassword, err
	}
	return &Session{dir: dir, key: key, env: *env}, data, method, nil
}

// openEnvelope finds the credential that opens this envelope.
//
// Both wraps are tried whichever the input looks like, and only the order changes: a recovery-shaped
// string is checked against the recovery wrap first because that is almost always what it is, and a
// master password that happens to look like a key still gets in on the second attempt. What must not
// happen is skipping a wrap based on the shape - that would turn a lucky-looking password into a
// failed unlock of a vault that would have opened.
//
// The single error for every failure is the point of this function. Reporting "no recovery key is
// set" or "that was almost a key" would tell someone holding a stolen vault file which half to
// spend their compute on.
func openEnvelope(env *envelope, credential string) (*age.X25519Identity, domain.UnlockMethod, error) {
	order := []wrapKind{wrapPassword, wrapRecovery}
	normalized := credential
	if key, ok := domain.NormalizeRecoveryKey(credential); ok {
		order = []wrapKind{wrapRecovery, wrapPassword}
		normalized = key
	}

	for _, kind := range order {
		wrapped, ok := env.wrapFor(kind)
		if !ok {
			continue
		}
		secret := credential
		method := domain.UnlockByPassword
		if kind == wrapRecovery {
			secret = normalized
			method = domain.UnlockByRecoveryKey
		}
		if key, err := unwrapVaultKey(wrapped, secret); err == nil {
			return key, method, nil
		}
	}
	return nil, domain.UnlockByPassword, fmt.Errorf("vault unlock: %w", domain.ErrVaultDecryptFailed)
}

// VerifyPassword reports whether password opens this vault's master password wrap, changing
// nothing and returning nothing.
//
// A recovery key that would open the vault is rejected. Callers use this to re-authenticate before
// something sensitive, and the recovery key is the credential most likely to be lying next to the
// machine on paper - accepting it here would make the re-authentication weaker than the unlock
// screen it is meant to be stricter than.
func VerifyPassword(dir, password string) error {
	raw, err := readVaultBytes(dir)
	if err != nil {
		return err
	}

	if IsLegacyAgeFile(raw) {
		_, err := DecryptLegacy(raw, password)
		return err
	}

	env, err := parseEnvelope(raw)
	if err != nil {
		return err
	}
	wrapped, ok := env.wrapFor(wrapPassword)
	if !ok {
		return fmt.Errorf("vault verify: %w", domain.ErrVaultDecryptFailed)
	}
	if _, err := unwrapVaultKey(wrapped, password); err != nil {
		return err
	}
	return nil
}

// ConvertLegacy upgrades a vault opened from the pre-envelope format, backing up the original first.
//
// The backup is named after the vault schema version inside the file rather than the envelope
// version, because that is the number that identifies which build can read it if the user ever has
// to go back to it.
func (s *Session) ConvertLegacy(masterPassword string, data *domain.VaultData) error {
	if !s.legacy {
		return nil
	}
	if err := BackupVaultFile(s.dir, data.Version); err != nil {
		return err
	}
	if err := s.Rekey(masterPassword); err != nil {
		return err
	}
	return s.Save(data)
}

// Rekey mints a fresh vault key and wraps it under the master password, discarding any wraps the
// session held. It writes nothing.
//
// The schema migration path uses this directly rather than ConvertLegacy, because it has already
// taken a backup under the version number it is leaving behind, and letting ConvertLegacy take a
// second one would file the pre-migration bytes under the version they are being migrated to.
func (s *Session) Rekey(masterPassword string) error {
	key, err := newVaultKey()
	if err != nil {
		return err
	}
	s.key = key
	s.env = envelope{Envelope: CurrentEnvelopeVersion}
	if err := s.SetPasswordWrap(masterPassword); err != nil {
		return err
	}
	s.legacy = false
	return nil
}

// Save encrypts the data to the vault key and atomically replaces the vault file.
func (s *Session) Save(data *domain.VaultData) error {
	if s.key == nil {
		return fmt.Errorf("vault save: %w", domain.ErrVaultLocked)
	}

	payload, err := encryptPayload(data, s.key)
	if err != nil {
		return err
	}
	s.env.Envelope = CurrentEnvelopeVersion
	s.env.Payload = payload

	raw, err := marshalEnvelope(&s.env)
	if err != nil {
		return err
	}
	return writeVaultBytes(s.dir, raw)
}

// SetPasswordWrap re-wraps the vault key under a master password, replacing any previous one.
func (s *Session) SetPasswordWrap(password string) error {
	return s.setWrap(wrapPassword, password)
}

// SetRecoveryWrap re-wraps the vault key under a recovery key, replacing any previous one.
//
// The key is normalized first, so what is stored matches what a user typing the printed form with
// their own spacing and case will produce.
func (s *Session) SetRecoveryWrap(recoveryKey string) error {
	normalized, ok := domain.NormalizeRecoveryKey(recoveryKey)
	if !ok {
		return fmt.Errorf("vault recovery wrap: %w", domain.ErrRecoveryKeyNotSet)
	}
	return s.setWrap(wrapRecovery, normalized)
}

func (s *Session) setWrap(kind wrapKind, credential string) error {
	if s.key == nil {
		return fmt.Errorf("vault wrap: %w", domain.ErrVaultLocked)
	}
	wrapped, err := wrapVaultKey(s.key, credential)
	if err != nil {
		return err
	}
	s.env.setWrap(kind, wrapped)
	return nil
}

// HasRecoveryWrap reports whether this vault currently has a recovery key that would open it.
func (s *Session) HasRecoveryWrap() bool {
	if s == nil {
		return false
	}
	_, ok := s.env.wrapFor(wrapRecovery)
	return ok
}

// IsLegacy reports whether this session came from a pre-envelope vault that ConvertLegacy has not
// upgraded yet.
func (s *Session) IsLegacy() bool {
	return s != nil && s.legacy
}
