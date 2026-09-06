package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"xquakshell/internal/domain"
)

// ReplicaSyncReport is what one synchronisation did, in terms the user can be shown.
//
// Conflicts and Removed are proposals rather than outcomes: an object edited on both devices keeps
// its local value, and one the remote no longer has is kept until the user says otherwise (I9).
type ReplicaSyncReport struct {
	Added     []string
	Updated   []string
	Conflicts []string
	Removed   []string
	Pushed    bool
}

// ReplicaSyncService synchronises one plugin's scope with whatever its transport can reach
// (ADR-022, port A).
//
// The order is fixed and each step exists for a reason: read what the remote has, decide which side
// is newer from the version vector sealed inside the ciphertext, apply only what belongs in the
// scope, then push the result back under the token that was read. Deciding from a version vector
// rather than a timestamp is what makes a hostile server unable to win an argument about which copy
// is newer - it never sees the vector, and cannot forge one it cannot read.
type ReplicaSyncService struct {
	vault     domain.VaultRepository
	settings  *PluginVaultSettings
	sealer    domain.ReplicaSealer
	transport domain.ReplicaTransport
}

// NewReplicaSyncService wires the pieces one synchronisation needs.
func NewReplicaSyncService(
	vault domain.VaultRepository,
	settings *PluginVaultSettings,
	sealer domain.ReplicaSealer,
	transport domain.ReplicaTransport,
) *ReplicaSyncService {
	return &ReplicaSyncService{vault: vault, settings: settings, sealer: sealer, transport: transport}
}

// Sync brings this device's copy of one plugin's scope into agreement with the remote.
//
// It fails rather than doing half the work. A sync that pushed without having applied, or applied
// without having pushed, leaves the version vector claiming a merge that did not happen - and the
// next sync then treats a divergence as settled.
func (s *ReplicaSyncService) Sync(ctx context.Context, pluginID string) (ReplicaSyncReport, error) {
	if s == nil || s.vault == nil || s.sealer == nil || s.transport == nil {
		return ReplicaSyncReport{}, fmt.Errorf("sync %s: replication is not wired", pluginID)
	}
	key, err := s.settings.ReplicaKey(ctx, pluginID)
	if err != nil {
		return ReplicaSyncReport{}, err
	}
	remote, token, err := s.fetch(ctx, pluginID, key)
	if err != nil {
		return ReplicaSyncReport{}, err
	}
	report, outgoing, err := s.reconcile(ctx, pluginID, remote)
	if err != nil {
		return ReplicaSyncReport{}, err
	}
	if err := s.push(ctx, pluginID, outgoing, key, token); err != nil {
		return ReplicaSyncReport{}, err
	}
	report.Pushed = true
	return report, nil
}

// fetch reads the remote and opens it, returning nothing at all when there is nothing stored yet.
//
// A remote with no bytes is the first device to synchronise, which is why it comes back as a nil
// document rather than an empty one: an empty document would merge as the other side having deleted
// everything, and that is precisely the message a fresh remote must not be able to send.
func (s *ReplicaSyncService) fetch(ctx context.Context, pluginID, key string) (*domain.ReplicaDocument, string, error) {
	sealed, token, err := s.transport.Fetch(ctx, pluginID)
	if err != nil {
		return nil, "", err
	}
	if len(sealed) == 0 {
		return nil, token, nil
	}
	opened, err := s.sealer.Open(sealed, key)
	if err != nil {
		return nil, "", fmt.Errorf("sync %s: %w", pluginID, err)
	}
	return &opened, token, nil
}

// reconcile applies what arrived and returns the document to send back.
//
// It runs inside one vault write. Reading the scope, merging, applying and recording the new
// version have to be one step: between any two of them the user can add a connection, and it would
// then be pushed under a version that says it was already there.
func (s *ReplicaSyncService) reconcile(
	ctx context.Context,
	pluginID string,
	remote *domain.ReplicaDocument,
) (ReplicaSyncReport, domain.ReplicaDocument, error) {
	var report ReplicaSyncReport
	var outgoing domain.ReplicaDocument
	err := s.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		deviceID, err := ensureDeviceID(data)
		if err != nil {
			return err
		}
		root, err := scopeRootFor(data, pluginID)
		if err != nil {
			return err
		}
		local, index, err := buildLocal(data, pluginID, root.Version)
		if err != nil {
			return err
		}
		merged, err := mergeWithRemote(local, remote)
		if err != nil {
			return fmt.Errorf("sync %s: %w", pluginID, err)
		}
		report.Conflicts, report.Removed = merged.Conflicts, merged.Removed
		if remote != nil {
			applied, err := domain.ApplyReplica(data, index, pluginID, merged.Result)
			if err != nil {
				return err
			}
			report.Added, report.Updated = applied.Added, applied.Updated
		}
		version := merged.Result.Version.Advance(deviceID)
		setScopeVersion(data, pluginID, version)
		outgoing, err = buildOutgoing(data, pluginID, version)
		return err
	})
	return report, outgoing, err
}

// push seals the result and stores it, if the remote still holds what was read.
func (s *ReplicaSyncService) push(ctx context.Context, pluginID string, doc domain.ReplicaDocument, key, token string) error {
	sealed, err := s.sealer.Seal(doc, key)
	if err != nil {
		return fmt.Errorf("sync %s: %w", pluginID, err)
	}
	if _, err := s.transport.Push(ctx, pluginID, sealed, token); err != nil {
		return err
	}
	return nil
}

// mergeWithRemote merges against what arrived, or reports the local side unchanged when nothing did.
//
// An empty remote is the first device to synchronise, not a deletion, so there is nothing to merge
// and nothing to apply.
func mergeWithRemote(local domain.ReplicaDocument, remote *domain.ReplicaDocument) (domain.ReplicaMerge, error) {
	if remote == nil {
		return domain.ReplicaMerge{Order: domain.ReplicaSame, Result: local}, nil
	}
	return domain.MergeReplica(local, *remote)
}

// buildLocal reads the scope as it stands, along with the index the apply will be checked against.
func buildLocal(data *domain.VaultData, pluginID string, version domain.VersionVector) (
	domain.ReplicaDocument, domain.ScopeIndex, error,
) {
	index, err := domain.NewScopeIndex(data.Folders, data.Settings.Plugins.ScopeRoots)
	if err != nil {
		return domain.ReplicaDocument{}, domain.ScopeIndex{}, fmt.Errorf("sync %s: %w", pluginID, err)
	}
	doc, err := domain.BuildReplicaDocument(data, index, pluginID, version)
	if err != nil {
		return domain.ReplicaDocument{}, domain.ScopeIndex{}, err
	}
	return doc, index, nil
}

// buildOutgoing re-reads the scope after the apply, so what is pushed is what this device now holds
// rather than the merge's idea of it.
func buildOutgoing(data *domain.VaultData, pluginID string, version domain.VersionVector) (domain.ReplicaDocument, error) {
	index, err := domain.NewScopeIndex(data.Folders, data.Settings.Plugins.ScopeRoots)
	if err != nil {
		return domain.ReplicaDocument{}, fmt.Errorf("sync %s: %w", pluginID, err)
	}
	return domain.BuildReplicaDocument(data, index, pluginID, version)
}

// scopeRootFor finds the folder this plugin synchronises.
//
// A plugin with no scope is refused rather than synchronised as empty: an empty document would tell
// the other devices that this one had deleted everything.
func scopeRootFor(data *domain.VaultData, pluginID string) (domain.ScopeRoot, error) {
	if data.Settings == nil {
		return domain.ScopeRoot{}, fmt.Errorf("sync %s: no settings in the vault", pluginID)
	}
	for _, root := range data.Settings.Plugins.ScopeRoots {
		if root.PluginID == pluginID {
			return root, nil
		}
	}
	return domain.ScopeRoot{}, fmt.Errorf("sync %s: the plugin has no scope", pluginID)
}

func setScopeVersion(data *domain.VaultData, pluginID string, version domain.VersionVector) {
	roots := data.Settings.Plugins.ScopeRoots
	for i := range roots {
		if roots[i].PluginID == pluginID {
			roots[i].Version = version
			return
		}
	}
}

// ensureDeviceID mints this installation's name in a version vector, once.
//
// Random rather than derived from anything about the machine: a hostile server sees it in no
// plaintext it holds, and there is nothing to learn from it if it ever leaks.
func ensureDeviceID(data *domain.VaultData) (string, error) {
	if data.Settings == nil {
		return "", fmt.Errorf("mint device id: no settings in the vault")
	}
	if id := data.Settings.Plugins.ReplicaDeviceID; id != "" {
		return id, nil
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("mint device id: %w", err)
	}
	data.Settings.Plugins.ReplicaDeviceID = hex.EncodeToString(raw)
	return data.Settings.Plugins.ReplicaDeviceID, nil
}
