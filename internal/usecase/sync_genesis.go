package usecase

import (
	"context"
	"io"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

// EnsureVaultMeta gives a vault its naming key if it has none yet: the first commit of every vault
// carries only the vault-meta record, so every later record id can be derived from it. On a vault
// that already has one it pulls and does nothing more, so a device that just logged in can call it
// unconditionally.
func (e *SyncEngine) EnsureVaultMeta(ctx context.Context, vault vaultsync.ID, random io.Reader) error {
	return e.ensureVaultMeta(ctx, vault, random, "")
}

func (e *SyncEngine) ensureVaultMeta(ctx context.Context, vault vaultsync.ID, random io.Reader, name string) error {
	p, err := e.startPull(ctx, vault)
	if err != nil {
		return err
	}
	if err := p.flushOutbox(ctx); err != nil {
		return err
	}
	if err := p.pullAll(ctx); err != nil {
		return err
	}
	if p.namingKey != nil {
		return nil
	}
	key := make([]byte, synctrust.NamingKeySize)
	if _, err := io.ReadFull(random, key); err != nil {
		return err
	}
	body, err := synctrust.NewTeamMetaBody(key, name)
	if err != nil {
		return err
	}
	_, err = p.commitAll(ctx, []change{{id: synctrust.VaultMetaID(vault), entry: synctrust.Entry{Type: synctrust.TypeVaultMeta, Data: body.Data}}}, nil)
	return err
}
