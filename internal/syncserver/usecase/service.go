package usecase

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"xquakshell/internal/domain/vaultsync"
)

// Config wires the service to its adapters.
type Config struct {
	Store  Store
	Clock  Clock
	AKE    vaultsync.PasswordAKEServer
	Random io.Reader
	// FakeAccountSecret keys the stable fake account ids handed out for unknown usernames.
	FakeAccountSecret []byte
	// PersonalQuota and TeamQuota bound the ciphertext bytes a vault may hold; zero means the
	// default (256 MiB and 1 GiB).
	PersonalQuota uint64
	TeamQuota     uint64
}

// Service is the sync server's application layer.
type Service struct {
	cfg        Config
	logins     *pendingLogins
	limits     *limiter
	challenges *challenges
	events     *hub
}

// New builds the service; in-memory state (pending logins, challenges, limits) starts empty.
func New(cfg Config) *Service {
	if cfg.PersonalQuota == 0 {
		cfg.PersonalQuota = 256 << 20
	}
	if cfg.TeamQuota == 0 {
		cfg.TeamQuota = 1 << 30
	}
	return &Service{cfg: cfg, logins: newPendingLogins(), limits: newLimiter(), challenges: newChallenges(), events: newHub()}
}

const (
	accountTokenTTL = 10 * 60_000
	accessTokenTTL  = 15 * 60_000
	tokenBytes      = 32
)

func (s *Service) now() uint64 { return s.cfg.Clock.NowUnixMilli() }

func (s *Service) randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(s.cfg.Random, b); err != nil {
		return nil, fmt.Errorf("usecase: randomness: %w", err)
	}
	return b, nil
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// issueToken creates a token, stores only its hash, and returns the value the client keeps.
func (s *Service) issueToken(tx Tx, account, device vaultsync.ID, kind TokenKind, ttl uint64) (string, uint64, error) {
	raw, err := s.randomBytes(tokenBytes)
	if err != nil {
		return "", 0, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expires := s.now() + ttl
	err = tx.InsertToken(Token{Hash: hashToken(token), Account: account, Device: device, Kind: kind, Expires: expires})
	return token, expires, err
}

// replayDeviceLog rebuilds an account's device log from its genesis keys and stored entries. The
// server trusts nothing it derived earlier: every authorization decision about a device starts from
// the signed log.
func replayDeviceLog(a Account, entries []vaultsync.DeviceEntry) (*vaultsync.DeviceLogState, error) {
	log, err := vaultsync.NewDeviceLogState(a.ID, vaultsync.Suite1, a.IdentityKey, a.EncryptionKey)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if err := log.Apply(&entries[i]); err != nil {
			return nil, fmt.Errorf("stored device log of %s: %w", a.ID, err)
		}
	}
	return log, nil
}
