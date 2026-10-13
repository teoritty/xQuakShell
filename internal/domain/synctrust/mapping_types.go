package synctrust

import (
	"fmt"
	"slices"

	"xquakshell/internal/domain"
)

// sliceMapper maps a vault slice whose items carry their own natural key. Upserting replaces the
// item with that key in place, keeping the user's order, or appends a new one.
func sliceMapper[T any](field func(*domain.VaultData) *[]T, keyOf func(T) string, validate func(*T) error) mapper {
	return mapper{
		entries: func(d *domain.VaultData) ([]Entry, error) {
			out := make([]Entry, 0, len(*field(d)))
			for _, v := range *field(d) {
				e, err := marshalEntry(keyOf(v), v)
				if err != nil {
					return nil, err
				}
				out = append(out, e)
			}
			return out, nil
		},
		keyOf: keyFrom(keyOf),
		upsert: func(key string, data []byte) (mutation, error) {
			v, err := decodeData[T](data)
			if err != nil {
				return nil, err
			}
			if err := checkKey(key, keyOf(v)); err != nil {
				return nil, err
			}
			if err := validate(&v); err != nil {
				return nil, fmt.Errorf("%w: %w", ErrRecordData, err)
			}
			return func(d *domain.VaultData) {
				s := field(d)
				if i := slices.IndexFunc(*s, func(x T) bool { return keyOf(x) == key }); i >= 0 {
					(*s)[i] = v
					return
				}
				*s = append(*s, v)
			}, nil
		},
		remove: func(key string) (mutation, error) {
			return func(d *domain.VaultData) {
				s := field(d)
				*s = slices.DeleteFunc(*s, func(x T) bool { return keyOf(x) == key })
			}, nil
		},
	}
}

func noValidation[T any](*T) error { return nil }

var folderMapper = sliceMapper(
	func(d *domain.VaultData) *[]domain.ConnectionFolder { return &d.Folders },
	func(f domain.ConnectionFolder) string { return f.ID },
	func(f *domain.ConnectionFolder) error { return f.Validate() },
)

var connectionMapper = sliceMapper(
	func(d *domain.VaultData) *[]domain.Connection { return &d.Connections },
	func(c domain.Connection) string { return c.ID },
	func(c *domain.Connection) error { return c.Validate() },
)

// The scope and subject are joined with a newline, which neither a plugin id nor a subject derived
// from a connection record can contain, so two different pairs never share a key.
var peerTrustMapper = sliceMapper(
	func(d *domain.VaultData) *[]domain.PeerTrustEntry { return &d.PeerTrust },
	func(e domain.PeerTrustEntry) string {
		if e.Scope == "" || e.Subject == "" {
			return ""
		}
		return e.Scope + "\n" + e.Subject
	},
	noValidation[domain.PeerTrustEntry],
)

type knownHostData struct {
	Line string `json:"line"`
}

func lineKey(l string) string { k, _ := knownHostKey(l); return k }

// knownHostMapper works on the raw lines. A malformed local line has no natural key and stays on
// this device rather than failing the whole snapshot. Should a vault hold two lines for one key, an
// incoming update replaces the first and drops the rest, so both devices end with the same one line.
var knownHostMapper = mapper{
	entries: func(d *domain.VaultData) ([]Entry, error) {
		var out []Entry
		seen := map[string]bool{}
		for _, l := range d.KnownHosts {
			k := lineKey(l)
			if k == "" || seen[k] {
				continue
			}
			seen[k] = true
			e, err := marshalEntry(k, knownHostData{Line: l})
			if err != nil {
				return nil, err
			}
			out = append(out, e)
		}
		return out, nil
	},
	keyOf: func(data []byte) (string, error) {
		v, err := decodeData[knownHostData](data)
		if err != nil {
			return "", err
		}
		return knownHostKey(v.Line)
	},
	upsert: func(key string, data []byte) (mutation, error) {
		v, err := decodeData[knownHostData](data)
		if err != nil {
			return nil, err
		}
		own, err := knownHostKey(v.Line)
		if err != nil {
			return nil, err
		}
		if err := checkKey(key, own); err != nil {
			return nil, err
		}
		return func(d *domain.VaultData) {
			i := slices.IndexFunc(d.KnownHosts, func(l string) bool { return lineKey(l) == key })
			if i < 0 {
				d.KnownHosts = append(d.KnownHosts, v.Line)
				return
			}
			d.KnownHosts[i] = v.Line
			rest := slices.DeleteFunc(d.KnownHosts[i+1:], func(l string) bool { return lineKey(l) == key })
			d.KnownHosts = d.KnownHosts[:i+1+len(rest)]
		}, nil
	},
	remove: func(key string) (mutation, error) {
		return func(d *domain.VaultData) {
			d.KnownHosts = slices.DeleteFunc(d.KnownHosts, func(l string) bool { return lineKey(l) == key })
		}, nil
	},
}

type identityData struct {
	Identity domain.SSHIdentity   `json:"identity"`
	Blob     *domain.IdentityBlob `json:"blob,omitempty"`
}

type passwordData struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Value []byte `json:"value"`
}

type pluginSecretData struct {
	Key   string `json:"key"`
	Value []byte `json:"value"`
}

// mapMapper maps a vault map whose items do not carry their own key; the record data adds it, so a
// receiving device can check it against the key the record was filed under.
func mapMapper[T any](list func(*domain.VaultData) []T, keyOf func(T) string, put func(*domain.VaultData, T), del func(*domain.VaultData, string)) mapper {
	return mapper{
		entries: func(d *domain.VaultData) ([]Entry, error) {
			var out []Entry
			for _, v := range list(d) {
				e, err := marshalEntry(keyOf(v), v)
				if err != nil {
					return nil, err
				}
				out = append(out, e)
			}
			return out, nil
		},
		keyOf: keyFrom(keyOf),
		upsert: func(key string, data []byte) (mutation, error) {
			v, err := decodeData[T](data)
			if err != nil {
				return nil, err
			}
			if err := checkKey(key, keyOf(v)); err != nil {
				return nil, err
			}
			return func(d *domain.VaultData) { put(d, v) }, nil
		},
		remove: func(key string) (mutation, error) {
			return func(d *domain.VaultData) { del(d, key) }, nil
		},
	}
}

var identityMapper = mapMapper(
	func(d *domain.VaultData) []identityData {
		var out []identityData
		for id, ident := range d.Identities {
			item := identityData{Identity: ident}
			if b, ok := d.KeyBlobs[id]; ok {
				item.Blob = &b
			}
			out = append(out, item)
		}
		return out
	},
	func(v identityData) string { return v.Identity.ID },
	func(d *domain.VaultData, v identityData) {
		ensureMaps(d)
		d.Identities[v.Identity.ID] = v.Identity
		if v.Blob == nil {
			delete(d.KeyBlobs, v.Identity.ID)
			return
		}
		d.KeyBlobs[v.Identity.ID] = *v.Blob
	},
	func(d *domain.VaultData, key string) { delete(d.Identities, key); delete(d.KeyBlobs, key) },
)

var passwordMapper = mapMapper(
	func(d *domain.VaultData) []passwordData {
		var out []passwordData
		for id, p := range d.Passwords {
			out = append(out, passwordData{ID: id, Label: p.Label, Value: p.Value})
		}
		return out
	},
	func(v passwordData) string { return v.ID },
	func(d *domain.VaultData, v passwordData) {
		ensureMaps(d)
		d.Passwords[v.ID] = domain.PasswordBlob{Value: v.Value, Label: v.Label}
	},
	func(d *domain.VaultData, key string) { delete(d.Passwords, key) },
)

var pluginSecretMapper = mapMapper(
	func(d *domain.VaultData) []pluginSecretData {
		var out []pluginSecretData
		for k, v := range d.PluginSecrets {
			out = append(out, pluginSecretData{Key: k, Value: v})
		}
		return out
	},
	func(v pluginSecretData) string { return v.Key },
	func(d *domain.VaultData, v pluginSecretData) {
		ensureMaps(d)
		d.PluginSecrets[v.Key] = v.Value
	},
	func(d *domain.VaultData, key string) { delete(d.PluginSecrets, key) },
)

// ensureMaps makes the vault's maps writable; a vault decoded from an older file may hold nil maps
// for fields it never had.
func ensureMaps(d *domain.VaultData) {
	if d.Identities == nil {
		d.Identities = map[string]domain.SSHIdentity{}
	}
	if d.KeyBlobs == nil {
		d.KeyBlobs = map[string]domain.IdentityBlob{}
	}
	if d.Passwords == nil {
		d.Passwords = map[string]domain.PasswordBlob{}
	}
	if d.PluginSecrets == nil {
		d.PluginSecrets = map[string][]byte{}
	}
}
