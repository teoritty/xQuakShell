package wails

import (
	"crypto/ed25519"
	"fmt"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/usecase"
)

// PluginInstallPreviewDTO describes install-time consent data.
type PluginInstallPreviewDTO struct {
	ID                           string   `json:"id"`
	Name                         string   `json:"name"`
	Version                      string   `json:"version"`
	Description                  string   `json:"description"`
	Signed                       bool     `json:"signed"`
	SignatureVerified            bool     `json:"signatureVerified"`
	ChecksumPresent              bool     `json:"checksumPresent"`
	RequiresSecretAccess         bool     `json:"requiresSecretAccess"`
	RequiresAuthProviderAccess   bool     `json:"requiresAuthProviderAccess"`
	RequiresTunnelProviderAccess bool     `json:"requiresTunnelProviderAccess"`
	MultiSessionWarning          bool     `json:"multiSessionWarning"`
	ArbitraryNetworkWarning      bool     `json:"arbitraryNetworkWarning"`
	ExecAccessWarning            bool     `json:"execAccessWarning"`
	UnsignedWarning              bool     `json:"unsignedWarning"`
	UntrustedSignatureWarning    bool     `json:"untrustedSignatureWarning"`
	Permissions                  []string `json:"permissions"`
}

func previewToDTO(p usecase.InstallPreview) PluginInstallPreviewDTO {
	return PluginInstallPreviewDTO{
		ID:                           p.ID,
		Name:                         p.Name,
		Version:                      p.Version,
		Description:                  p.Description,
		Signed:                       p.Signed,
		SignatureVerified:            p.SignatureVerified,
		ChecksumPresent:              p.ChecksumPresent,
		RequiresSecretAccess:         p.RequiresSecretAccess,
		RequiresAuthProviderAccess:   p.RequiresAuthProviderAccess,
		RequiresTunnelProviderAccess: p.RequiresTunnelProviderAccess,
		MultiSessionWarning:          p.MultiSessionWarning,
		ArbitraryNetworkWarning:      p.ArbitraryNetworkWarning,
		ExecAccessWarning:            p.ExecAccessWarning,
		UnsignedWarning:              p.UnsignedWarning,
		UntrustedSignatureWarning:    p.UntrustedSignatureWarning,
		Permissions:                  p.Permissions,
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

// PluginSettingsDTO is the part of the plugin settings section the settings dialog owns.
//
// It is deliberately not the whole section. The capability grants and the disabled-plugin list are
// established by install-time consent, never travel here, and are merged back by the caller — a DTO
// that carried them would let a settings save rewrite decisions the user made somewhere else.
type PluginSettingsDTO struct {
	TrustedPublisherKeys []string `json:"trustedPublisherKeys"`
	RequireSignedPlugins bool     `json:"requireSignedPlugins"`
	// AllowUnsandboxedFallback lets a plugin start unconfined when this platform CAN confine it and
	// the attempt failed. It does not affect a platform that cannot confine at all, where plugins
	// start unconfined regardless.
	AllowUnsandboxedFallback bool `json:"allowUnsandboxedFallback"`
}

func pluginSettingsToDTO(s domain.PluginSettings) PluginSettingsDTO {
	keys := s.TrustedPublisherKeys
	if keys == nil {
		keys = []string{}
	}
	return PluginSettingsDTO{
		TrustedPublisherKeys:     keys,
		RequireSignedPlugins:     s.RequireSignedPlugins,
		AllowUnsandboxedFallback: s.AllowUnsandboxedFallback,
	}
}

// dtoToPluginSettings folds the dialog's three fields onto the stored section, leaving everything
// else exactly as it was. Building a fresh struct here instead would clear every capability grant
// and the disabled-plugin list on each save.
func dtoToPluginSettings(dto PluginSettingsDTO, current domain.PluginSettings) domain.PluginSettings {
	keys := dto.TrustedPublisherKeys
	if keys == nil {
		keys = []string{}
	}
	merged := current
	merged.TrustedPublisherKeys = keys
	merged.RequireSignedPlugins = dto.RequireSignedPlugins
	merged.AllowUnsandboxedFallback = dto.AllowUnsandboxedFallback
	return merged
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
	return a.settingsSvc.SavePluginSettings(a.reqCtx(), dtoToPluginSettings(dto, settings.Plugins))
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
