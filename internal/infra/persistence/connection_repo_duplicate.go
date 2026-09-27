package persistence

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"xquakshell/internal/domain"
)

// Duplicate stores an independent copy of a connection, placed directly after the original.
//
// The copy and the secrets it needs are written in one vault update. Done as a save followed by a
// second update, a failure between the two would leave a copy whose secret fields point at
// references that were never filled, and connecting with it would fail with "secret not found"
// for no reason the user could see.
func (r *ConnectionRepo) Duplicate(ctx context.Context, sourceID, name string, newRuleID func() (string, error)) (*domain.Connection, error) {
	var created domain.Connection
	err := r.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		index := connectionIndex(data.Connections, sourceID)
		if index < 0 {
			return fmt.Errorf("duplicate connection %s: %w", sourceID, domain.ErrConnectionNotFound)
		}
		src := data.Connections[index]
		dup, err := domain.DuplicateConnection(src, uuid.New().String(), name, newRuleID)
		if err != nil {
			return err
		}
		if err := copyPluginSecrets(data, dup.SecretCopies); err != nil {
			return err
		}
		makeRoomAfter(data.Connections, src)
		dup.Connection.Order = src.Order + 1
		data.Connections = append(data.Connections, dup.Connection)
		created = domain.CloneConnection(dup.Connection)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
}

func connectionIndex(connections []domain.Connection, id string) int {
	for i := range connections {
		if connections[i].ID == id {
			return i
		}
	}
	return -1
}

// A reference with no stored value is refused rather than skipped: the copy would claim a secret
// it does not have, and the source is already broken in the same way.
//
// Every source is checked before anything is written. UpdateData mutates the live data in place
// and keeps whatever a failed mutation already changed, so a copy that failed on its second secret
// would otherwise leave the first behind under an id no connection has.
func copyPluginSecrets(data *domain.VaultData, copies map[string]string) error {
	for _, source := range copies {
		if _, ok := data.PluginSecrets[source]; !ok {
			return fmt.Errorf("duplicate connection: secret %s is missing: %w", source, domain.ErrInvalidConnectionConfig)
		}
	}
	for target, source := range copies {
		data.PluginSecrets[target] = append([]byte(nil), data.PluginSecrets[source]...)
	}
	return nil
}

// makeRoomAfter shifts the siblings that sort after src down by one, so the copy lands directly
// beneath the original rather than at one end of the folder, where it would have to be found.
func makeRoomAfter(connections []domain.Connection, src domain.Connection) {
	for i := range connections {
		if connections[i].FolderID == src.FolderID && connections[i].Order > src.Order {
			connections[i].Order++
		}
	}
}
