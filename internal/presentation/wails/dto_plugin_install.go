package wails

import (
	"context"
	"crypto/ed25519"
	"fmt"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/domain"
	"ssh-client/internal/usecase"
)

// PluginInstallPreviewDTO describes install-time consent data.
type PluginInstallPreviewDTO struct {
	ID                        string   `json:"id"`
	Name                      string   `json:"name"`
	Version                   string   `json:"version"`
	Description               string   `json:"description"`
	Signed                    bool     `json:"signed"`
	SignatureVerified         bool     `json:"signatureVerified"`
	ChecksumPresent           bool     `json:"checksumPresent"`
	RequiresSecretAccess      bool     `json:"requiresSecretAccess"`
	MultiSessionWarning       bool     `json:"multiSessionWarning"`
	UnsignedWarning           bool     `json:"unsignedWarning"`
	UntrustedSignatureWarning bool     `json:"untrustedSignatureWarning"`
	Permissions               []string `json:"permissions"`
}

func previewToDTO(p usecase.InstallPreview) PluginInstallPreviewDTO {
	return PluginInstallPreviewDTO{
		ID:                        p.ID,
		Name:                      p.Name,
		Version:                   p.Version,
		Description:               p.Description,
		Signed:                    p.Signed,
		SignatureVerified:         p.SignatureVerified,
		ChecksumPresent:           p.ChecksumPresent,
		RequiresSecretAccess:      p.RequiresSecretAccess,
		MultiSessionWarning:       p.MultiSessionWarning,
		UnsignedWarning:           p.UnsignedWarning,
		UntrustedSignatureWarning: p.UntrustedSignatureWarning,
		Permissions:               p.Permissions,
	}
}

func (a *AppAPI) pluginTrustPolicy() (domainplugin.InstallTrustPolicy, error) {
	policy := domainplugin.InstallTrustPolicy{}
	if a.settingsSvc == nil {
		return policy, nil
	}
	settings, err := a.settingsSvc.GetSettings()
	if err != nil {
		return policy, err
	}
	policy.RequireSigned = settings.Plugins.RequireSignedPlugins
	keys, err := domainplugin.ParseTrustedPublisherKeys(settings.Plugins.TrustedPublisherKeys)
	if err != nil {
		return policy, err
	}
	policy.TrustedKeys = keys
	return policy, nil
}

// PluginSettingsDTO is the plugin section of application settings.
type PluginSettingsDTO struct {
	TrustedPublisherKeys []string `json:"trustedPublisherKeys"`
	RequireSignedPlugins bool     `json:"requireSignedPlugins"`
}

func pluginSettingsToDTO(s domain.PluginSettings) PluginSettingsDTO {
	keys := s.TrustedPublisherKeys
	if keys == nil {
		keys = []string{}
	}
	return PluginSettingsDTO{
		TrustedPublisherKeys: keys,
		RequireSignedPlugins: s.RequireSignedPlugins,
	}
}

func dtoToPluginSettings(dto PluginSettingsDTO) domain.PluginSettings {
	keys := dto.TrustedPublisherKeys
	if keys == nil {
		keys = []string{}
	}
	return domain.PluginSettings{
		TrustedPublisherKeys: keys,
		RequireSignedPlugins: dto.RequireSignedPlugins,
	}
}

// GetPluginSettings returns plugin trust/install settings.
func (a *AppAPI) GetPluginSettings() (PluginSettingsDTO, error) {
	if a.settingsSvc == nil {
		return PluginSettingsDTO{TrustedPublisherKeys: []string{}}, nil
	}
	settings, err := a.settingsSvc.GetSettings()
	if err != nil {
		return PluginSettingsDTO{}, err
	}
	return pluginSettingsToDTO(settings.Plugins), nil
}

// SavePluginSettings persists plugin trust/install settings.
func (a *AppAPI) SavePluginSettings(dto PluginSettingsDTO) error {
	if a.settingsSvc == nil {
		return fmt.Errorf("settings unavailable")
	}
	if _, err := domainplugin.ParseTrustedPublisherKeys(dto.TrustedPublisherKeys); err != nil {
		return err
	}
	settings, err := a.settingsSvc.GetSettings()
	if err != nil {
		return err
	}
	settings.Plugins = dtoToPluginSettings(dto)
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.settingsSvc.SaveSettings(ctx, settings)
}

// GeneratePluginPublisherKeyPair returns a new Ed25519 key pair for plugin signing.
func (a *AppAPI) GeneratePluginPublisherKeyPair() (PluginPublisherKeyPairDTO, error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return PluginPublisherKeyPairDTO{}, err
	}
	return PluginPublisherKeyPairDTO{
		PublicKey:  domainplugin.EncodePublicKey(pub),
		PrivateKey: domainplugin.EncodePrivateKey(priv),
	}, nil
}

// PluginPublisherKeyPairDTO holds base64 Ed25519 keys for plugin authors.
type PluginPublisherKeyPairDTO struct {
	PublicKey  string `json:"publicKey"`
	PrivateKey string `json:"privateKey"`
}

// ValidateTrustedPublisherKey checks that a base64 string is a valid Ed25519 public key.
func (a *AppAPI) ValidateTrustedPublisherKey(keyB64 string) error {
	_, err := domainplugin.ParseTrustedPublisherKeys([]string{keyB64})
	return err
}
