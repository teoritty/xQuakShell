package usecase

import (
	"context"

	"xquakshell/internal/domain"
)

// VaultService orchestrates CRUD for vault-stored folders, connections, passwords, and identities.
// Single orchestration entry for vault CRUD from presentation; do not call repositories from handlers.
type VaultService struct {
	connRepo             domain.ConnectionRepository
	passwordRepo         domain.PasswordRepository
	identRepo            domain.IdentityRepository
	pluginFields         *PluginFieldsService
	pingMgr              *PingManager
	protocolLookup       domain.ConnectionProtocolLookup
	forwardRuleValidator *ForwardRuleValidator
}

type VaultServiceConfig struct {
	ConnRepo       domain.ConnectionRepository
	PasswordRepo   domain.PasswordRepository
	IdentRepo      domain.IdentityRepository
	PluginFields   *PluginFieldsService
	PingMgr        *PingManager
	ProtocolLookup domain.ConnectionProtocolLookup
}

func NewVaultService(cfg VaultServiceConfig) *VaultService {
	if cfg.ConnRepo == nil {
		panic("usecase: VaultService requires ConnectionRepository")
	}
	if cfg.PasswordRepo == nil {
		panic("usecase: VaultService requires PasswordRepository")
	}
	if cfg.IdentRepo == nil {
		panic("usecase: VaultService requires IdentityRepository")
	}
	return &VaultService{
		connRepo:       cfg.ConnRepo,
		passwordRepo:   cfg.PasswordRepo,
		identRepo:      cfg.IdentRepo,
		pluginFields:   cfg.PluginFields,
		pingMgr:        cfg.PingMgr,
		protocolLookup: cfg.ProtocolLookup,
	}
}

func (s *VaultService) GetAllFolders(ctx context.Context) ([]domain.ConnectionFolder, error) {
	return s.connRepo.GetAllFolders(ctx)
}

func (s *VaultService) SaveFolder(ctx context.Context, folder *domain.ConnectionFolder) error {
	return s.connRepo.SaveFolder(ctx, folder)
}

func (s *VaultService) DeleteFolder(ctx context.Context, id string) error {
	return s.connRepo.DeleteFolder(ctx, id)
}

func (s *VaultService) ImportPassword(ctx context.Context, password []byte, label string) (string, error) {
	return s.passwordRepo.Import(ctx, password, label)
}

func (s *VaultService) DeletePassword(ctx context.Context, id string) error {
	return s.passwordRepo.Delete(ctx, id)
}

func (s *VaultService) SetForwardRuleValidator(v *ForwardRuleValidator) {
	if s == nil {
		return
	}
	s.forwardRuleValidator = v
}

func (s *VaultService) GetAllIdentities(ctx context.Context) ([]domain.SSHIdentity, error) {
	return s.identRepo.GetAll(ctx)
}

func (s *VaultService) ImportIdentity(ctx context.Context, pemData []byte, comment string) (*domain.SSHIdentity, error) {
	return s.identRepo.Import(ctx, pemData, comment)
}
