package usecase

import "xquakshell/internal/domain"

// checkHopScope holds a team connection's jump hops to the same rule as its users: a hop never
// takes a key or password from outside the connection's vault. Hops have no binding, so a reference
// another member wrote there would otherwise be looked up in this member's own vault and used.
func (c *SSHConnector) checkHopScope(conn *domain.Connection, hop domain.JumpHop) error {
	if conn.VaultID == "" {
		return nil
	}
	data, err := c.vaultRepo.GetData()
	if err != nil {
		return err
	}
	return domain.CheckCredentialScope(data, conn.VaultID, hop.KeyAuth, hop.PassAuth)
}
