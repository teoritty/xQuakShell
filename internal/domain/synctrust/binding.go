package synctrust

import (
	"reflect"

	"xquakshell/internal/domain"
)

// AddressChanged reports whether a connection version points somewhere else than the one before:
// another host, port or jump chain. Other members' credentials bound to the connection must not
// follow such a change unconfirmed (spec §5.9). Data that does not decode counts as changed.
func AddressChanged(before, after []byte) bool {
	a, errA := decodeData[domain.Connection](before)
	b, errB := decodeData[domain.Connection](after)
	if errA != nil || errB != nil {
		return true
	}
	return a.Host != b.Host || a.Port != b.Port || !sameJumps(a.JumpChain, b.JumpChain)
}

// sameJumps compares jump chains, treating an absent and an empty chain alike: two builds may
// encode "no hops" differently, and that is no repointing.
func sameJumps(a, b domain.JumpChainConfig) bool {
	if len(a.Hops) == 0 && len(b.Hops) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

// SuspendBindings suspends this member's bindings to one connection of one team vault, naming the
// account whose change suspended them.
func SuspendBindings(d *domain.VaultData, vault, connection, byAccount string) {
	if d.Sync == nil {
		return
	}
	for i := range d.Sync.Bindings {
		b := &d.Sync.Bindings[i]
		if b.VaultID == vault && b.ConnectionID == connection {
			b.SuspendedBy = byAccount
		}
	}
}
