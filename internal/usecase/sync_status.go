package usecase

import (
	"bytes"
	"errors"
	"slices"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

// The scheduler reports each vault on its own, but a person reads one status for the account. The
// last report alone was that status once, and it lied: a team vault that synced fine after the
// personal vault stopped said "up to date", and the counts of held and quarantined changes were
// those of whichever vault reported last.

// stateRank orders states by how much they need the person; the account shows the worst.
func stateRank(s SyncState) int {
	switch s {
	case SyncStopped:
		return 4
	case SyncFailed:
		return 3
	case SyncRunning:
		return 2
	case SyncIdle:
		return 1
	}
	return 0
}

// accountLevel reports an error that is about the account rather than one vault. When the keys
// moved on or the device was removed every vault fails, and whatever a vault reports besides is a
// consequence: the person must sign in again or let the device go before anything else can verify.
func accountLevel(err error) bool {
	return errors.Is(err, ErrSyncDeviceRevoked) || errors.Is(err, synctrust.ErrKeysRotated)
}

// rollupStatus folds the last status of every vault into the account's. Held and quarantined
// changes add up; the state is the worst any vault is in; the error is an account-level one if any
// vault has one, else that of the worst vault, the personal vault first and then in id order so the
// same set of reports always shows the same message.
func rollupStatus(per map[vaultsync.ID]SyncStatus, personal vaultsync.ID) SyncStatus {
	ids := make([]vaultsync.ID, 0, len(per))
	for id := range per {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b vaultsync.ID) int {
		if (a == personal) != (b == personal) {
			if a == personal {
				return -1
			}
			return 1
		}
		return bytes.Compare(a[:], b[:])
	})
	out := SyncStatus{Vault: personal}
	var worst error
	for _, id := range ids {
		st := per[id]
		out.Report.Pull.Held += st.Report.Pull.Held
		out.Report.Pull.Quarantined += st.Report.Pull.Quarantined
		if stateRank(st.State) > stateRank(out.State) {
			out.State, worst = st.State, st.Err
		}
		if st.Err != nil && accountLevel(st.Err) && !accountLevel(out.Err) {
			out.Err = st.Err
		}
	}
	if out.Err == nil {
		out.Err = worst
	}
	return out
}
