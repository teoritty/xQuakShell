package usecase

import (
	"bytes"
	"errors"
	"fmt"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

// interpretation is what one verified commit means for the local vault.
type interpretation struct {
	changes    []synctrust.Entry
	applied    []vaultsync.ID
	quarantine []synctrust.QuarantineItem
}

// errSkip marks a record with nothing to apply: a tombstone for something this device never had.
var errSkip = errors.New("nothing to apply")

// interpret decides each record of a verified commit. The vault-meta record goes first, because
// every other record's id is checked against the naming key it carries.
func (p *pullRun) interpret(c vaultsync.Commit) (interpretation, error) {
	metaID := synctrust.VaultMetaID(p.vault)
	for _, r := range c.Records {
		if r.Record == metaID {
			if err := p.learnMeta(r); err != nil {
				return interpretation{}, err
			}
		}
	}
	local, err := p.localSnapshot()
	if err != nil {
		return interpretation{}, err
	}
	var out interpretation
	for i := range c.Records {
		r := &c.Records[i]
		if r.Record == metaID {
			continue
		}
		entry, reason := p.decode(r)
		if errors.Is(reason, errSkip) {
			continue
		}
		if reason != nil {
			out.quarantine = append(out.quarantine, p.hold(r, reason.Error(), false))
			continue
		}
		p.route(&out, r, entry, local[r.Record].Data)
	}
	return out, nil
}

func (p *pullRun) route(out *interpretation, r *vaultsync.Envelope, entry synctrust.Entry, local []byte) {
	entry, ok := p.mergeLocal(out, r.Record, entry, local)
	if !ok {
		return
	}
	verdict, err := synctrust.Classify(entry.Type, local, entry.Data)
	switch {
	case err != nil:
		out.quarantine = append(out.quarantine, p.hold(r, "the record's data does not decode", false))
	case verdict.Action == synctrust.ActionApply:
		out.changes = append(out.changes, entry)
		out.applied = append(out.applied, r.Record)
	case verdict.Action == synctrust.ActionPending:
		out.quarantine = append(out.quarantine, p.hold(r, verdict.Reason, true))
	default:
		out.quarantine = append(out.quarantine, p.hold(r, verdict.Reason, false))
	}
}

func (p *pullRun) hold(r *vaultsync.Envelope, reason string, pending bool) synctrust.QuarantineItem {
	return synctrust.QuarantineItem{Envelope: *r, Reason: reason, AtUnixMilli: p.e.d.NowUnixMilli(), Pending: pending}
}

func (p *pullRun) localSnapshot() (map[vaultsync.ID]synctrust.Entry, error) {
	if p.namingKey == nil {
		return nil, nil
	}
	data, err := p.e.d.Vault.GetData()
	if err != nil {
		return nil, err
	}
	return synctrust.Snapshot(p.namingKey, data)
}

// learnMeta reads the vault's naming key. A vault-meta that is deleted, does not open, or names a
// different key from the one already known is an integrity failure: it would re-point every
// record id in the vault.
func (p *pullRun) learnMeta(r vaultsync.Envelope) error {
	if r.Deleted {
		return integrity("vault-meta", fmt.Errorf("deleted"))
	}
	body, err := p.open(&r)
	if err != nil {
		return integrity("vault-meta", err)
	}
	key, err := synctrust.VaultMetaNamingKey(body)
	if err != nil {
		return integrity("vault-meta", err)
	}
	if p.namingKey != nil && !bytes.Equal(p.namingKey, key) {
		return integrity("vault-meta", fmt.Errorf("the naming key changed"))
	}
	p.namingKey = key
	return nil
}

func (p *pullRun) open(r *vaultsync.Envelope) (synctrust.Body, error) {
	vk, err := p.e.d.Keys.VaultKey(p.vault, r.Epoch, p.ops)
	if err != nil {
		return synctrust.Body{}, fmt.Errorf("no key for epoch %d: %w", r.Epoch, err)
	}
	pt, err := p.e.d.Cipher.Open(vk, r)
	if err != nil {
		return synctrust.Body{}, fmt.Errorf("the record does not decrypt: %w", err)
	}
	return synctrust.DecodeBody(pt)
}

// decode turns one record into a vault change, or into the reason it cannot be one. The reasons
// are shown to the user in the quarantine list, so they say what is wrong, not how it was found.
func (p *pullRun) decode(r *vaultsync.Envelope) (synctrust.Entry, error) {
	if p.namingKey == nil {
		return synctrust.Entry{}, errors.New("arrived before the vault's naming key")
	}
	if r.Deleted {
		name, ok := p.nameOf(r.Record)
		if !ok {
			return synctrust.Entry{}, errSkip
		}
		return synctrust.Entry{Type: name.ty, Key: name.key}, nil
	}
	body, err := p.open(r)
	if err != nil {
		return synctrust.Entry{}, errors.New("the record does not decrypt or decode")
	}
	switch vaultsync.RecordAccess(synctrust.KnownSchemas(), string(body.Type), body.Schema) {
	case vaultsync.SchemaOpaque:
		return synctrust.Entry{}, errors.New("a record type this version does not know")
	case vaultsync.SchemaReadOnly:
		return synctrust.Entry{}, errors.New("written by a newer version of xQuakShell")
	}
	return p.checkFiled(r, body)
}

// checkFiled requires the record to sit under the id its own natural key derives, so one device
// cannot overwrite an item through another item's record, and passes the domain validators.
func (p *pullRun) checkFiled(r *vaultsync.Envelope, body synctrust.Body) (synctrust.Entry, error) {
	key, err := synctrust.NaturalKey(body.Type, body.Data)
	if err != nil {
		return synctrust.Entry{}, errors.New("the record names no valid item")
	}
	id, err := synctrust.RecordID(p.namingKey, body.Type, key)
	if err != nil || id != r.Record {
		return synctrust.Entry{}, errors.New("filed under another item's record")
	}
	entry := synctrust.Entry{Type: body.Type, Key: key, Data: body.Data}
	if err := synctrust.Validate(entry); err != nil {
		return synctrust.Entry{}, errors.New("the item fails validation")
	}
	p.names[r.Record] = syncKeyName{ty: body.Type, key: key}
	return entry, nil
}

// nameOf recovers which item a record held, for a tombstone that carries nothing: from this pull,
// or by opening the version this device verified before.
func (p *pullRun) nameOf(id vaultsync.ID) (syncKeyName, bool) {
	if n, ok := p.names[id]; ok {
		return n, true
	}
	prev, ok := p.base[id]
	if !ok || prev.Deleted {
		return syncKeyName{}, false
	}
	body, err := p.open(&prev)
	if err != nil {
		return syncKeyName{}, false
	}
	key, err := synctrust.NaturalKey(body.Type, body.Data)
	if err != nil {
		return syncKeyName{}, false
	}
	return syncKeyName{ty: body.Type, key: key}, true
}
