package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"ssh-client/internal/domain"
)

// PuTTYSessionPreview is a parsed PuTTY session suitable for UI preview.
type PuTTYSessionPreview struct {
	Name     string
	HostName string
	Port     int
	UserName string
}

// PuTTYImportService imports PuTTY artifacts into the vault.
type PuTTYImportService struct {
	connRepo  domain.ConnectionRepository
	identRepo domain.IdentityRepository
	importer  domain.PuTTYImporter
}

// NewPuTTYImportService creates a PuTTY import service.
func NewPuTTYImportService(connRepo domain.ConnectionRepository, identRepo domain.IdentityRepository, importer domain.PuTTYImporter) *PuTTYImportService {
	return &PuTTYImportService{
		connRepo:  connRepo,
		identRepo: identRepo,
		importer:  importer,
	}
}

// ParseReg parses a PuTTY .reg export into session previews.
func (s *PuTTYImportService) ParseReg(regContent string) ([]PuTTYSessionPreview, error) {
	if s == nil || s.importer == nil {
		return nil, fmt.Errorf("putty importer unavailable")
	}
	sessions, err := s.importer.ParseReg(regContent)
	if err != nil {
		return nil, err
	}
	out := make([]PuTTYSessionPreview, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, PuTTYSessionPreview{
			Name:     session.Name,
			HostName: session.HostName,
			Port:     session.Port,
			UserName: session.UserName,
		})
	}
	return out, nil
}

// ImportPPK imports a PuTTY .ppk file into the vault as an identity.
func (s *PuTTYImportService) ImportPPK(ctx context.Context, ppkData []byte, passphrase string) (string, error) {
	if s == nil || s.identRepo == nil || s.importer == nil {
		return "", fmt.Errorf("identity repository unavailable")
	}
	pemData, comment, err := s.importer.PPKToPEM(ppkData, passphrase)
	if err != nil {
		return "", err
	}
	if comment == "" {
		comment = "PuTTY import"
	}
	identity, err := s.identRepo.Import(ctx, pemData, comment)
	if err != nil {
		return "", err
	}
	return identity.ID, nil
}

// ImportRegAsConnections parses a PuTTY .reg export and creates vault connections.
func (s *PuTTYImportService) ImportRegAsConnections(ctx context.Context, regContent, folderID string) ([]domain.Connection, error) {
	if s == nil || s.connRepo == nil || s.importer == nil {
		return nil, fmt.Errorf("connection repository unavailable")
	}
	sessions, err := s.importer.ParseReg(regContent)
	if err != nil {
		return nil, err
	}
	var result []domain.Connection
	for i, session := range sessions {
		if session.HostName == "" {
			continue
		}
		conn := puttySessionToConnection(session, folderID, i)
		conn.ID = ""
		if err := s.connRepo.Save(ctx, &conn); err != nil {
			return result, fmt.Errorf("save session %s: %w", session.Name, err)
		}
		result = append(result, conn)
	}
	return result, nil
}

func puttySessionToConnection(s domain.PuTTYSession, folderID string, order int) domain.Connection {
	conn := domain.Connection{
		FolderID: folderID,
		Name:     s.Name,
		Host:     s.HostName,
		Port:     s.Port,
		Order:    order,
		Protocol: domain.ProtocolSSH,
	}
	if conn.Port == 0 {
		conn.Port = domain.DefaultSSHPort
	}
	if s.UserName != "" {
		uid := "u-" + randomHex(4)
		conn.Users = []domain.ConnectionUser{
			{ID: uid, Username: s.UserName, Auth: domain.AuthMethodKey},
		}
		conn.DefaultUserID = uid
	}
	return conn
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b)
}
