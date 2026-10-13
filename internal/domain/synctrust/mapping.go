package synctrust

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/vaultsync"
)

// Entry is the plaintext meaning of one record: its type, its natural key, and its data. A nil Data
// is a deletion of that natural key.
type Entry struct {
	Type RecordType
	Key  string
	Data json.RawMessage
}

var (
	// ErrRecordData is a record whose data does not decode, fails the domain validator for its type,
	// or names a different natural key than the one it was filed under.
	ErrRecordData = errors.New("invalid record data")
	// ErrRecordType is a record type this build does not map onto the vault.
	ErrRecordType = errors.New("unmapped record type")
	// ErrDuplicateKey is a local vault holding two items with one natural key, which would file two
	// different records under one id.
	ErrDuplicateKey = errors.New("two items share one record key")
)

// mutation is a validated change waiting to be applied. Apply prepares every change before running
// any, so a commit lands whole or not at all.
type mutation func(*domain.VaultData)

type mapper struct {
	entries func(d *domain.VaultData) ([]Entry, error)
	keyOf   func(data []byte) (string, error)
	upsert  func(key string, data []byte) (mutation, error)
	remove  func(key string) (mutation, error)
}

var mappers = map[RecordType]mapper{
	TypeFolder:       folderMapper,
	TypeConnection:   connectionMapper,
	TypeIdentity:     identityMapper,
	TypePassword:     passwordMapper,
	TypePluginSecret: pluginSecretMapper,
	TypeKnownHost:    knownHostMapper,
	TypePeerTrust:    peerTrustMapper,
}

// mapperFor finds the mapper for t. vault-meta and conflict records have none on purpose: they are
// the engine's own records and never become part of the vault the user edits.
func mapperFor(t RecordType) (mapper, string, error) {
	if section, ok := IsSettingsType(t); ok {
		return settingsMapper(section), section, nil
	}
	if m, ok := mappers[t]; ok {
		return m, "", nil
	}
	return mapper{}, "", fmt.Errorf("%w: %q", ErrRecordType, t)
}

// Snapshot lists every synced item of d, filed under its record id.
func Snapshot(namingKey []byte, d *domain.VaultData) (map[vaultsync.ID]Entry, error) {
	out := map[vaultsync.ID]Entry{}
	add := func(e Entry) error {
		id, err := RecordID(namingKey, e.Type, e.Key)
		if err != nil {
			return err
		}
		if _, dup := out[id]; dup {
			return fmt.Errorf("%w: %s %q", ErrDuplicateKey, e.Type, e.Key)
		}
		out[id] = e
		return nil
	}
	types := append(append([]RecordType{}, schemaV1...), settingsTypes()...)
	for _, t := range types {
		m, _, err := mapperFor(t)
		if err != nil {
			continue
		}
		entries, err := m.entries(d)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			e.Type = t
			if err := add(e); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func settingsTypes() []RecordType {
	out := make([]RecordType, 0, len(settingsSections))
	for _, s := range settingsSections {
		out = append(out, SettingsType(s))
	}
	return out
}

// Apply validates every change against the domain rules for its type and then applies them all to d.
// If any change is refused, d is left untouched.
func Apply(d *domain.VaultData, changes []Entry) error {
	muts := make([]mutation, 0, len(changes))
	for _, c := range changes {
		m, _, err := mapperFor(c.Type)
		if err != nil {
			return err
		}
		var mut mutation
		if c.Data == nil {
			mut, err = m.remove(c.Key)
		} else {
			mut, err = m.upsert(c.Key, c.Data)
		}
		if err != nil {
			return fmt.Errorf("%s %q: %w", c.Type, c.Key, err)
		}
		muts = append(muts, mut)
	}
	for _, mut := range muts {
		mut(d)
	}
	return nil
}

// NaturalKey reads the natural key a record's data names. The engine derives the record id from it
// and refuses a record filed under any other id.
func NaturalKey(t RecordType, data []byte) (string, error) {
	m, _, err := mapperFor(t)
	if err != nil {
		return "", err
	}
	return m.keyOf(data)
}

// Validate runs the same checks Apply would for one change, without applying it, so the engine can
// quarantine a single bad record instead of refusing the whole commit it arrived in.
func Validate(e Entry) error {
	m, _, err := mapperFor(e.Type)
	if err != nil {
		return err
	}
	if e.Data == nil {
		_, err = m.remove(e.Key)
	} else {
		_, err = m.upsert(e.Key, e.Data)
	}
	return err
}

// keyFrom builds a mapper's keyOf from its data type and key function.
func keyFrom[T any](keyOf func(T) string) func([]byte) (string, error) {
	return func(data []byte) (string, error) {
		v, err := decodeData[T](data)
		if err != nil {
			return "", err
		}
		if k := keyOf(v); k != "" {
			return k, nil
		}
		return "", fmt.Errorf("%w: no natural key", ErrRecordData)
	}
}

// decodeData parses record data strictly; an unknown field means a newer schema or a hostile
// device, and the schema number already routes the first case away from here.
func decodeData[T any](raw []byte) (T, error) {
	var v T
	if err := decodeStrict(raw, &v); err != nil {
		return v, fmt.Errorf("%w: %w", ErrRecordData, err)
	}
	return v, nil
}

func checkKey(filed, own string) error {
	if own == "" || filed != own {
		return fmt.Errorf("%w: data names key %q, filed under %q", ErrRecordData, own, filed)
	}
	return nil
}

func marshalEntry(key string, v any) (Entry, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return Entry{}, fmt.Errorf("encode %q: %w", key, err)
	}
	return Entry{Key: key, Data: raw}, nil
}

func settingsMapper(section string) mapper {
	return mapper{
		entries: func(d *domain.VaultData) ([]Entry, error) {
			if d.Settings == nil {
				return nil, nil
			}
			raw, err := sectionCodecs[section].encode(*d.Settings)
			if err != nil {
				return nil, fmt.Errorf("encode settings %s: %w", section, err)
			}
			return []Entry{{Key: section, Data: raw}}, nil
		},
		keyOf: func(data []byte) (string, error) {
			if _, err := ApplySection(domain.AppSettings{}, section, data); err != nil {
				return "", fmt.Errorf("%w: %w", ErrRecordData, err)
			}
			return section, nil
		},
		upsert: func(key string, data []byte) (mutation, error) {
			if err := checkKey(section, key); err != nil {
				return nil, err
			}
			if _, err := ApplySection(domain.AppSettings{}, section, data); err != nil {
				return nil, fmt.Errorf("%w: %w", ErrRecordData, err)
			}
			return func(d *domain.VaultData) {
				if d.Settings == nil {
					d.Settings = domain.NewVaultData().Settings
				}
				// Validated above against the same codec; it cannot fail here.
				s, _ := ApplySection(*d.Settings, section, data)
				*d.Settings = s
			}, nil
		},
		remove: func(string) (mutation, error) {
			return nil, fmt.Errorf("%w: a settings section is never deleted", ErrRecordData)
		},
	}
}

// knownHostKey is the natural key of an OpenSSH known_hosts line: its marker if any, its host
// pattern and its key type - everything but the key. A new key for the same host and type is then
// an update of the same record, which is what lets Classify see a replaced host key at all.
func knownHostKey(line string) (string, error) {
	f := strings.Fields(line)
	n := 2
	if len(f) > 0 && strings.HasPrefix(f[0], "@") {
		n = 3
	}
	if len(f) < n+1 {
		return "", fmt.Errorf("%w: malformed known_hosts line", ErrRecordData)
	}
	return strings.Join(f[:n], " "), nil
}
