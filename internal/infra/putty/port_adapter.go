package putty

import "ssh-client/internal/domain"

// PortAdapter implements domain.PuTTYImporter using the infra putty package.
type PortAdapter struct{}

// ParseReg implements domain.PuTTYImporter.
func (PortAdapter) ParseReg(content string) ([]domain.PuTTYSession, error) {
	sessions, err := ParsePuTTYReg(content)
	if err != nil {
		return nil, err
	}
	out := make([]domain.PuTTYSession, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, domain.PuTTYSession{
			Name:     s.Name,
			HostName: s.HostName,
			Port:     s.Port,
			UserName: s.UserName,
			Protocol: s.Protocol,
		})
	}
	return out, nil
}

// PPKToPEM implements domain.PuTTYImporter.
func (PortAdapter) PPKToPEM(ppkContent []byte, passphrase string) ([]byte, string, error) {
	return PPKToPEM(ppkContent, passphrase)
}

// PuttySessionToConnection converts a domain PuTTY session to a vault connection.
func PuttySessionToConnection(s domain.PuTTYSession, folderID string, order int) domain.Connection {
	p := PuTTYSession{
		Name:     s.Name,
		HostName: s.HostName,
		Port:     s.Port,
		UserName: s.UserName,
		Protocol: s.Protocol,
	}
	return p.ToConnection(folderID, order)
}
