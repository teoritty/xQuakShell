package usecase

import (
	"context"
	"errors"
	"fmt"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

// SyncAll synchronises every installed plugin that declared replication, and reports how many it
// managed.
//
// It runs once per unlock, which is the first moment there is a key to seal with and a vault to
// read. One plugin's failure does not cost the others theirs: a single unreachable server would
// otherwise stop every other scope from ever converging, and the reasons a sync fails - a laptop
// with no network, a server being restarted - are exactly the ones that affect one plugin and not
// the rest.
//
// A plugin with no replication key is skipped rather than reported. That is the ordinary state of a
// freshly installed sync plugin: the user has not yet minted a key or typed in the one from their
// other device, and there is nothing wrong for the unlock path to shout about.
func (s *ReplicaSyncService) SyncAll(ctx context.Context, installed []domainplugin.InstalledPlugin) (int, error) {
	var synced int
	var firstErr error
	for i := range installed {
		manifest := &installed[i].Manifest
		if !manifest.Replicates() {
			continue
		}
		if err := ctx.Err(); err != nil {
			return synced, fmt.Errorf("sync plugins: %w", err)
		}
		switch _, err := s.Sync(ctx, manifest.ID); {
		case errors.Is(err, domain.ErrReplicaKeyRequired):
			continue
		case err != nil:
			if firstErr == nil {
				firstErr = err
			}
		default:
			synced++
		}
	}
	return synced, firstErr
}
