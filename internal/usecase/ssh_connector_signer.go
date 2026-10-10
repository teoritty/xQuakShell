package usecase

import (
	"context"

	"xquakshell/internal/domain"
)

var _ AgentSignerSource = (*SSHConnector)(nil)

// SignerFor opens a vault key for a forwarded-agent signature by the same path a login takes:
// the key's own policy, the passphrase cache, and the passphrase dialog with its retry limit. A
// second path for the agent would be a second set of caching rules to get wrong.
func (c *SSHConnector) SignerFor(ctx context.Context, identityID string) (domain.Signer, error) {
	return c.signerFor(ctx, identityID)
}
