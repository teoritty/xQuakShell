package wails

import (
	"sort"
	"strings"

	"xquakshell/internal/domain"
)

type FolderDTO struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ParentID string `json:"parentId"`
	Order    int    `json:"order"`
}

type PluginAuthConfigDTO struct {
	PluginID     string            `json:"pluginId"`
	AuthMethodID string            `json:"authMethodId"`
	Fields       map[string]string `json:"fields,omitempty"`
}

type ConnectionUserDTO struct {
	ID         string               `json:"id"`
	Username   string               `json:"username"`
	Auth       string               `json:"authMethod"`
	KeyAuth    *KeyAuthConfigDTO    `json:"keyAuth,omitempty"`
	PassAuth   *PassAuthConfigDTO   `json:"passAuth,omitempty"`
	PluginAuth *PluginAuthConfigDTO `json:"pluginAuth,omitempty"`
	Label      string               `json:"label,omitempty"`
}

type KeyAuthConfigDTO struct {
	IdentityIDs []string `json:"identityIds"`
}

// PassAuthConfigDTO carries the vault handle for password auth, never the password.
// See domain.PasswordAuthConfig for why the field is VaultRef and the tag is not.
type PassAuthConfigDTO struct {
	VaultRef string `json:"passwordId"`
}

type JumpHopDTO struct {
	ID         string               `json:"id"`
	Host       string               `json:"host"`
	Port       int                  `json:"port"`
	Username   string               `json:"username"`
	Auth       string               `json:"authMethod"`
	KeyAuth    *KeyAuthConfigDTO    `json:"keyAuth,omitempty"`
	PassAuth   *PassAuthConfigDTO   `json:"passAuth,omitempty"`
	PluginAuth *PluginAuthConfigDTO `json:"pluginAuth,omitempty"`
}

type ForwardRuleDTO struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	BindAddress string `json:"bindAddress"`
	BindPort    int    `json:"bindPort"`
	TargetHost  string `json:"targetHost,omitempty"`
	TargetPort  int    `json:"targetPort,omitempty"`
	PluginID    string `json:"pluginId,omitempty"`
	ProviderID  string `json:"providerId,omitempty"`
	Enabled     bool   `json:"enabled"`
	// AllowRemoteGateway carries the user's acknowledgement that a remote forward may bind beyond
	// the SSH server's loopback. Without it domain.ForwardRule.Validate refuses such a rule.
	AllowRemoteGateway bool `json:"allowRemoteGateway,omitempty"`
}

type ConnectionDTO struct {
	ID            string              `json:"id"`
	FolderID      string              `json:"folderId"`
	Name          string              `json:"name"`
	Host          string              `json:"host"`
	Port          int                 `json:"port"`
	Order         int                 `json:"order"`
	Protocol      string              `json:"protocol,omitempty"`
	Users         []ConnectionUserDTO `json:"users,omitempty"`
	DefaultUserID string              `json:"defaultUserId,omitempty"`
	Tags          []string            `json:"tags,omitempty"`
	JumpChain     []JumpHopDTO        `json:"jumpChain,omitempty"`
	PluginFields  map[string]string   `json:"pluginFields,omitempty"`
	// StoredSecretFields lists plugin field ids whose secret value is already stored in the vault.
	// PluginFields carries no entry at all for them (a secret is never sent to the UI, and an empty
	// value would read as "clear it" on the way back), so the form uses this to show a "saved"
	// placeholder and to leave the field out of the save payload while untouched.
	StoredSecretFields []string         `json:"storedSecretFields,omitempty"`
	ForwardRules       []ForwardRuleDTO `json:"forwardRules,omitempty"`
}

type KnownHostDTO struct {
	Host        string `json:"host"`
	KeyType     string `json:"keyType"`
	Fingerprint string `json:"fingerprint"`
}

func FolderToDTO(f domain.ConnectionFolder) FolderDTO {
	return FolderDTO{ID: f.ID, Name: f.Name, ParentID: f.ParentID, Order: f.Order}
}

func FoldersToDTO(fs []domain.ConnectionFolder) []FolderDTO {
	result := make([]FolderDTO, len(fs))
	for i, f := range fs {
		result[i] = FolderToDTO(f)
	}
	return result
}

func ConnectionToDTO(c domain.Connection) ConnectionDTO {
	dto := ConnectionDTO{
		ID:            c.ID,
		FolderID:      c.FolderID,
		Name:          c.Name,
		Host:          c.Host,
		Port:          c.Port,
		Order:         c.Order,
		Protocol:      c.GetProtocol(),
		DefaultUserID: c.DefaultUserID,
		Tags:          cloneStringSlice(c.Tags),
	}
	if dto.Tags == nil {
		dto.Tags = []string{}
	}
	for _, u := range c.Users {
		dto.Users = append(dto.Users, connectionUserToDTO(u))
	}
	for _, h := range c.JumpChain.Hops {
		dto.JumpChain = append(dto.JumpChain, jumpHopToDTO(h))
	}
	for _, r := range c.ForwardRules {
		dto.ForwardRules = append(dto.ForwardRules, forwardRuleToDTO(r))
	}
	dto.PluginFields = pluginFieldsToDTO(c.PluginFields)
	dto.StoredSecretFields = storedSecretFieldIDs(c.PluginFields)
	return dto
}

// storedSecretFieldIDs returns the plugin field ids that carry a stored secret reference, sorted
// for a stable payload. pluginFieldsToDTO leaves these out of the DTO, so this list is the only
// thing that tells the editor a secret is on file.
func storedSecretFieldIDs(fields map[string]string) []string {
	var ids []string
	for k, v := range fields {
		if strings.HasPrefix(v, "secret:") {
			ids = append(ids, k)
		}
	}
	sort.Strings(ids)
	return ids
}

func connectionUserToDTO(u domain.ConnectionUser) ConnectionUserDTO {
	d := ConnectionUserDTO{
		ID:       u.ID,
		Username: u.Username,
		Auth:     string(u.Auth),
		Label:    u.Label,
	}
	if u.KeyAuth != nil {
		d.KeyAuth = keyAuthToDTO(u.KeyAuth)
	}
	if u.PassAuth != nil {
		d.PassAuth = passAuthToDTO(u.PassAuth)
	}
	if u.PluginAuth != nil {
		d.PluginAuth = pluginAuthToDTO(u.PluginAuth)
	}
	return d
}

func pluginAuthToDTO(in *domain.PluginAuthConfig) *PluginAuthConfigDTO {
	if in == nil {
		return nil
	}
	return &PluginAuthConfigDTO{
		PluginID:     in.PluginID,
		AuthMethodID: in.AuthMethodID,
		Fields:       cloneStringMap(in.Fields),
	}
}

func forwardRuleToDTO(r domain.ForwardRule) ForwardRuleDTO {
	return ForwardRuleDTO{
		ID:          r.ID,
		Kind:        string(r.Kind),
		BindAddress: r.BindAddress,
		BindPort:    r.BindPort,
		TargetHost:  r.TargetHost,
		TargetPort:  r.TargetPort,
		PluginID:    r.PluginID,
		ProviderID:  r.ProviderID,
		Enabled:     r.Enabled,

		AllowRemoteGateway: r.AllowRemoteGateway,
	}
}

func jumpHopToDTO(h domain.JumpHop) JumpHopDTO {
	d := JumpHopDTO{
		ID:       h.ID,
		Host:     h.Host,
		Port:     h.Port,
		Username: h.Username,
		Auth:     string(h.Auth),
	}
	if h.KeyAuth != nil {
		d.KeyAuth = keyAuthToDTO(h.KeyAuth)
	}
	if h.PassAuth != nil {
		d.PassAuth = passAuthToDTO(h.PassAuth)
	}
	if h.PluginAuth != nil {
		d.PluginAuth = pluginAuthToDTO(h.PluginAuth)
	}
	return d
}

func keyAuthToDTO(in *domain.KeyAuthConfig) *KeyAuthConfigDTO {
	if in == nil {
		return nil
	}
	return &KeyAuthConfigDTO{IdentityIDs: cloneStringSlice(in.IdentityIDs)}
}

func passAuthToDTO(in *domain.PasswordAuthConfig) *PassAuthConfigDTO {
	if in == nil {
		return nil
	}
	return &PassAuthConfigDTO{VaultRef: in.VaultRef}
}

func keyAuthFromDTO(in *KeyAuthConfigDTO) *domain.KeyAuthConfig {
	if in == nil {
		return nil
	}
	return &domain.KeyAuthConfig{IdentityIDs: cloneStringSlice(in.IdentityIDs)}
}

func passAuthFromDTO(in *PassAuthConfigDTO) *domain.PasswordAuthConfig {
	if in == nil {
		return nil
	}
	return &domain.PasswordAuthConfig{VaultRef: in.VaultRef}
}

func pluginAuthFromDTO(in *PluginAuthConfigDTO) *domain.PluginAuthConfig {
	if in == nil {
		return nil
	}
	return &domain.PluginAuthConfig{
		PluginID:     in.PluginID,
		AuthMethodID: in.AuthMethodID,
		Fields:       cloneStringMap(in.Fields),
	}
}

func forwardRuleFromDTO(d ForwardRuleDTO) domain.ForwardRule {
	return domain.ForwardRule{
		ID:          d.ID,
		Kind:        domain.ForwardRuleKind(d.Kind),
		BindAddress: d.BindAddress,
		BindPort:    d.BindPort,
		TargetHost:  d.TargetHost,
		TargetPort:  d.TargetPort,
		PluginID:    d.PluginID,
		ProviderID:  d.ProviderID,
		Enabled:     d.Enabled,

		AllowRemoteGateway: d.AllowRemoteGateway,
	}
}

func cloneStringSlice(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func ConnectionsToDTO(cs []domain.Connection) []ConnectionDTO {
	result := make([]ConnectionDTO, len(cs))
	for i, c := range cs {
		result[i] = ConnectionToDTO(c)
	}
	return result
}

func KnownHostToDTO(e domain.KnownHostEntry) KnownHostDTO {
	return KnownHostDTO{Host: e.Host, KeyType: e.KeyType, Fingerprint: e.Fingerprint}
}

func KnownHostsToDTO(es []domain.KnownHostEntry) []KnownHostDTO {
	result := make([]KnownHostDTO, len(es))
	for i, e := range es {
		result[i] = KnownHostToDTO(e)
	}
	return result
}

func DTOToFolder(d FolderDTO) domain.ConnectionFolder {
	return domain.ConnectionFolder{ID: d.ID, Name: d.Name, ParentID: d.ParentID, Order: d.Order}
}

func DTOToConnection(d ConnectionDTO) domain.Connection {
	c := domain.Connection{
		ID:            d.ID,
		FolderID:      d.FolderID,
		Name:          d.Name,
		Host:          d.Host,
		Port:          d.Port,
		Order:         d.Order,
		Protocol:      d.Protocol,
		DefaultUserID: d.DefaultUserID,
		Tags:          cloneStringSlice(d.Tags),
	}
	for _, u := range d.Users {
		c.Users = append(c.Users, dtoToConnectionUser(u))
	}
	for _, h := range d.JumpChain {
		c.JumpChain.Hops = append(c.JumpChain.Hops, dtoToJumpHop(h))
	}
	for _, r := range d.ForwardRules {
		c.ForwardRules = append(c.ForwardRules, forwardRuleFromDTO(r))
	}
	if len(d.PluginFields) > 0 {
		c.PluginFields = cloneStringMap(d.PluginFields)
	}
	return c
}

func dtoToConnectionUser(d ConnectionUserDTO) domain.ConnectionUser {
	u := domain.ConnectionUser{
		ID:       d.ID,
		Username: d.Username,
		Auth:     domain.AuthMethodType(d.Auth),
		Label:    d.Label,
	}
	u.KeyAuth = keyAuthFromDTO(d.KeyAuth)
	u.PassAuth = passAuthFromDTO(d.PassAuth)
	u.PluginAuth = pluginAuthFromDTO(d.PluginAuth)
	return u
}

func dtoToJumpHop(d JumpHopDTO) domain.JumpHop {
	h := domain.JumpHop{
		ID:       d.ID,
		Host:     d.Host,
		Port:     d.Port,
		Username: d.Username,
		Auth:     domain.AuthMethodType(d.Auth),
	}
	h.KeyAuth = keyAuthFromDTO(d.KeyAuth)
	h.PassAuth = passAuthFromDTO(d.PassAuth)
	h.PluginAuth = pluginAuthFromDTO(d.PluginAuth)
	return h
}

// pluginFieldsToDTO drops every field holding a vault secret reference instead of masking it to "".
//
// An empty value is not a neutral placeholder here: SavePluginFields reads it as "the user emptied
// this field" and either refuses the save (the field is required) or deletes the stored secret (it
// is not). A mask therefore cannot survive a round trip, and the UI does round-trip a connection -
// the tree's inline rename saves back the record it was handed. Absence is the value that means
// "unchanged", and storedSecretFields already tells the editor which secrets exist.
func pluginFieldsToDTO(fields map[string]string) map[string]string {
	out := make(map[string]string, len(fields))
	for k, v := range fields {
		if strings.HasPrefix(v, "secret:") {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
