package wails

import (
	"xquakshell/internal/domain"
	"xquakshell/internal/usecase"
)

// PlanKeyMigration says whether the vault needs upgrading and which keys the upgrade will ask
// about, changing nothing on disk.
//
// The frontend calls this when an unlock reports that a migration is required, which is why the
// answer is a structure rather than a matched error string: "needs upgrading" and "wrong password"
// lead to opposite screens and must not be told apart by comparing message text.
func (a *AppAPI) PlanKeyMigration(masterPassword string) (MigrationPlanDTO, error) {
	if a.migrator == nil {
		return MigrationPlanDTO{}, domain.ErrVaultNotFound
	}
	plan, err := a.migrator.PlanMigration(a.reqCtx(), masterPassword, a.migrationDeps)
	if err != nil {
		return MigrationPlanDTO{}, err
	}
	out := MigrationPlanDTO{Required: plan.Required, Keys: make([]PendingKeyDTO, 0, len(plan.Keys))}
	for _, key := range plan.Keys {
		out.Keys = append(out.Keys, PendingKeyDTO{ID: key.ID, Comment: key.Comment, KeyType: key.KeyType})
	}
	return out, nil
}

// CompleteKeyMigration upgrades the vault and leaves it unlocked.
//
// answers maps identity id to passphrase. A key the user chose to skip is simply absent, which is
// what stops a forgotten passphrase from making the whole vault unopenable.
func (a *AppAPI) CompleteKeyMigration(masterPassword string, answers map[string]string) (MigrationReportDTO, error) {
	if a.migrator == nil {
		return MigrationReportDTO{}, domain.ErrVaultNotFound
	}
	report, err := a.migrator.CompleteMigration(a.reqCtx(), masterPassword, answers, a.migrationDeps)
	if err != nil {
		return MigrationReportDTO{}, err
	}
	return MigrationReportDTO{
		FromVersion: report.FromVersion,
		ToVersion:   report.ToVersion,
		Converted:   report.Converted,
		Skipped:     report.Skipped,
		BackupPath:  report.BackupPath,
	}, nil
}

// wireSessionsAndKeys installs the session manager together with the key services.
//
// They are wired in one call because key publication needs a live session's SFTP channel, so it
// cannot be built before the session manager exists and there is no useful moment between the two.
//
// The vault repository is also the migrator; the type assertion fails only for a test double that
// does not implement the upgrade, and both migration handlers refuse cleanly when it is absent
// rather than panicking the moment a user tries to open an old vault.
func (a *AppAPI) wireSessionsAndKeys(
	sessions *usecase.SessionManager,
	vaultRepo domain.VaultRepository,
	sshSession usecase.SSHSessionDeps,
	auditLogRepo domain.AuditLogRepository,
) {
	a.sessions = sessions
	a.keys = sshSession.Keys
	a.migrationDeps = sshSession.MigrationDeps
	if migrator, ok := vaultRepo.(domain.VaultMigrator); ok {
		a.migrator = migrator
	}
	a.keyDeploy = usecase.NewKeyDeployService(sessions, sshSession.Keys, usecase.NewKeyAuditRecorder(auditLogRepo))
}
