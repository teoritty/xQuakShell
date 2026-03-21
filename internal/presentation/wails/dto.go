package wails

import "ssh-client/internal/domain"

// FolderDTO is the UI-facing representation of a folder.
type FolderDTO struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ParentID string `json:"parentId"`
	Order    int    `json:"order"`
}

// ConnectionUserDTO is the UI-facing representation of a connection user.
type ConnectionUserDTO struct {
	ID       string             `json:"id"`
	Username string             `json:"username"`
	Auth     string             `json:"authMethod"`
	KeyAuth  *KeyAuthConfigDTO  `json:"keyAuth,omitempty"`
	PassAuth *PassAuthConfigDTO `json:"passAuth,omitempty"`
	Label    string             `json:"label,omitempty"`
}

// KeyAuthConfigDTO holds key auth references for the UI.
type KeyAuthConfigDTO struct {
	IdentityIDs []string `json:"identityIds"`
}

// PassAuthConfigDTO holds password auth reference for the UI.
type PassAuthConfigDTO struct {
	PasswordID string `json:"passwordId"`
}

// JumpHopDTO is the UI-facing representation of a single jump hop.
type JumpHopDTO struct {
	Host     string             `json:"host"`
	Port     int                `json:"port"`
	Username string             `json:"username"`
	Auth     string             `json:"authMethod"`
	KeyAuth  *KeyAuthConfigDTO  `json:"keyAuth,omitempty"`
	PassAuth *PassAuthConfigDTO `json:"passAuth,omitempty"`
}

// ProxyDTO is the UI-facing representation of SOCKS proxy config.
type ProxyDTO struct {
	Type       string `json:"type"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username,omitempty"`
	PasswordID string `json:"passwordId,omitempty"`
}

// TelnetConfigDTO is the UI-facing representation of Telnet config.
type TelnetConfigDTO struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username,omitempty"`
	PasswordID string `json:"passwordId,omitempty"`
}

// RDPConfigDTO is the UI-facing representation of RDP config.
type RDPConfigDTO struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username,omitempty"`
	PasswordID string `json:"passwordId,omitempty"`
	Domain     string `json:"domain,omitempty"`
}

// SerialConfigDTO is the UI-facing representation of Serial config.
type SerialConfigDTO struct {
	Port     string `json:"port"`
	BaudRate int    `json:"baudRate"`
	DataBits int    `json:"dataBits"`
	StopBits int    `json:"stopBits"`
	Parity   string `json:"parity"`
}

// HTTPConfigDTO is the UI-facing representation of HTTP config.
type HTTPConfigDTO struct {
	URL        string `json:"url"`
	Method     string `json:"method"`
	Auth       string `json:"auth,omitempty"`
	PasswordID string `json:"passwordId,omitempty"`
}

// ConnectionDTO is the UI-facing representation of a connection.
type ConnectionDTO struct {
	ID            string              `json:"id"`
	FolderID      string              `json:"folderId"`
	Name          string              `json:"name"`
	Host          string              `json:"host"`
	Port          int                 `json:"port"`
	Order         int                 `json:"order"`
	Protocol      string              `json:"protocol,omitempty"`
	User          string              `json:"user,omitempty"`
	IdentityIDs   []string            `json:"identityIds,omitempty"`
	Users         []ConnectionUserDTO `json:"users,omitempty"`
	DefaultUserID string              `json:"defaultUserId,omitempty"`
	Tags          []string            `json:"tags,omitempty"`
	VpnProfileID  string              `json:"vpnProfileId,omitempty"`
	JumpChain     []JumpHopDTO        `json:"jumpChain,omitempty"`
	Proxy         *ProxyDTO           `json:"proxy,omitempty"`
	TelnetConfig  *TelnetConfigDTO    `json:"telnetConfig,omitempty"`
	RDPConfig     *RDPConfigDTO       `json:"rdpConfig,omitempty"`
	SerialConfig  *SerialConfigDTO    `json:"serialConfig,omitempty"`
	HTTPConfig    *HTTPConfigDTO      `json:"httpConfig,omitempty"`
}

// IdentityDTO is the UI-facing representation of an SSH identity.
type IdentityDTO struct {
	ID        string `json:"id"`
	Comment   string `json:"comment"`
	KeyType   string `json:"keyType"`
	Encrypted bool   `json:"encrypted"`
}

// SessionDTO is the UI-facing representation of a session.
type SessionDTO struct {
	SessionID      string `json:"sessionId"`
	ConnectionID   string `json:"connectionId"`
	ConnectionName string `json:"connectionName"`
	Protocol       string `json:"protocol,omitempty"`
	State          string `json:"state"`
	ErrorMessage   string `json:"errorMessage"`
}

// RemoteNodeDTO is the UI-facing representation of a remote file/directory.
type RemoteNodeDTO struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	ModTime string `json:"modTime"`
	Mode    string `json:"mode,omitempty"`
	Owner   string `json:"owner,omitempty"`
	Group   string `json:"group,omitempty"`
}

// KnownHostDTO is the UI-facing representation of a known host entry.
type KnownHostDTO struct {
	Host        string `json:"host"`
	KeyType     string `json:"keyType"`
	Fingerprint string `json:"fingerprint"`
}

// --- Mapping functions ---

// FolderToDTO maps a domain folder to a DTO.
func FolderToDTO(f domain.ConnectionFolder) FolderDTO {
	return FolderDTO{
		ID: f.ID, Name: f.Name, ParentID: f.ParentID, Order: f.Order,
	}
}

// FoldersToDTO maps a slice of domain folders to DTOs.
func FoldersToDTO(fs []domain.ConnectionFolder) []FolderDTO {
	result := make([]FolderDTO, len(fs))
	for i, f := range fs {
		result[i] = FolderToDTO(f)
	}
	return result
}

// ConnectionToDTO maps a domain connection to a DTO.
func ConnectionToDTO(c domain.Connection) ConnectionDTO {
	dto := ConnectionDTO{
		ID:            c.ID,
		FolderID:      c.FolderID,
		Name:          c.Name,
		Host:          c.Host,
		Port:          c.Port,
		Order:         c.Order,
		Protocol:      c.GetProtocol(),
		User:          c.User,
		IdentityIDs:   c.IdentityIDs,
		DefaultUserID: c.DefaultUserID,
		Tags:          c.Tags,
		VpnProfileID:  c.VpnProfileID,
	}
	if dto.IdentityIDs == nil {
		dto.IdentityIDs = []string{}
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
	if c.Proxy != nil && !c.Proxy.IsEmpty() {
		dto.Proxy = &ProxyDTO{
			Type:       c.Proxy.Type,
			Host:       c.Proxy.Host,
			Port:       c.Proxy.Port,
			Username:   c.Proxy.Username,
			PasswordID: c.Proxy.PasswordID,
		}
	}
	if c.TelnetConfig != nil {
		dto.TelnetConfig = &TelnetConfigDTO{
			Host:       c.TelnetConfig.Host,
			Port:       c.TelnetConfig.Port,
			Username:   c.TelnetConfig.Username,
			PasswordID: c.TelnetConfig.PasswordID,
		}
	}
	if c.RDPConfig != nil {
		dto.RDPConfig = &RDPConfigDTO{
			Host:       c.RDPConfig.Host,
			Port:       c.RDPConfig.Port,
			Username:   c.RDPConfig.Username,
			PasswordID: c.RDPConfig.PasswordID,
			Domain:     c.RDPConfig.Domain,
		}
	}
	if c.SerialConfig != nil {
		dto.SerialConfig = &SerialConfigDTO{
			Port:     c.SerialConfig.Port,
			BaudRate: c.SerialConfig.BaudRate,
			DataBits: c.SerialConfig.DataBits,
			StopBits: c.SerialConfig.StopBits,
			Parity:   c.SerialConfig.Parity,
		}
	}
	if c.HTTPConfig != nil {
		dto.HTTPConfig = &HTTPConfigDTO{
			URL:        c.HTTPConfig.URL,
			Method:     c.HTTPConfig.Method,
			Auth:       c.HTTPConfig.Auth,
			PasswordID: c.HTTPConfig.PasswordID,
		}
	}
	return dto
}

func connectionUserToDTO(u domain.ConnectionUser) ConnectionUserDTO {
	d := ConnectionUserDTO{
		ID:       u.ID,
		Username: u.Username,
		Auth:     string(u.Auth),
		Label:    u.Label,
	}
	if u.KeyAuth != nil {
		d.KeyAuth = &KeyAuthConfigDTO{IdentityIDs: u.KeyAuth.IdentityIDs}
	}
	if u.PassAuth != nil {
		d.PassAuth = &PassAuthConfigDTO{PasswordID: u.PassAuth.PasswordID}
	}
	return d
}

func jumpHopToDTO(h domain.JumpHop) JumpHopDTO {
	d := JumpHopDTO{
		Host:     h.Host,
		Port:     h.Port,
		Username: h.Username,
		Auth:     string(h.Auth),
	}
	if h.KeyAuth != nil {
		d.KeyAuth = &KeyAuthConfigDTO{IdentityIDs: h.KeyAuth.IdentityIDs}
	}
	if h.PassAuth != nil {
		d.PassAuth = &PassAuthConfigDTO{PasswordID: h.PassAuth.PasswordID}
	}
	return d
}

// ConnectionsToDTO maps a slice of domain connections to DTOs.
func ConnectionsToDTO(cs []domain.Connection) []ConnectionDTO {
	result := make([]ConnectionDTO, len(cs))
	for i, c := range cs {
		result[i] = ConnectionToDTO(c)
	}
	return result
}

// IdentityToDTO maps a domain identity to a DTO.
func IdentityToDTO(id domain.SSHIdentity) IdentityDTO {
	return IdentityDTO{
		ID: id.ID, Comment: id.Comment, KeyType: id.KeyType, Encrypted: id.Encrypted,
	}
}

// IdentitiesToDTO maps a slice of domain identities to DTOs.
func IdentitiesToDTO(ids []domain.SSHIdentity) []IdentityDTO {
	result := make([]IdentityDTO, len(ids))
	for i, id := range ids {
		result[i] = IdentityToDTO(id)
	}
	return result
}

// SessionToDTO maps a domain session to a DTO.
func SessionToDTO(s domain.ConnectionSession) SessionDTO {
	return SessionDTO{
		SessionID:      s.SessionID,
		ConnectionID:   s.ConnectionID,
		ConnectionName: s.ConnectionName,
		Protocol:       s.Protocol,
		State:          string(s.State),
		ErrorMessage:   s.ErrorMessage,
	}
}

// RemoteNodeToDTO maps a domain remote node to a DTO.
func RemoteNodeToDTO(n domain.RemoteNode) RemoteNodeDTO {
	return RemoteNodeDTO{
		Path:    n.Path,
		Name:    n.Name,
		IsDir:   n.IsDir,
		Size:    n.Size,
		ModTime: n.ModTime.Format("2006-01-02 15:04:05"),
		Mode:    n.Mode,
		Owner:   n.Owner,
		Group:   n.Group,
	}
}

// RemoteNodesToDTO maps a slice of domain remote nodes to DTOs.
func RemoteNodesToDTO(ns []domain.RemoteNode) []RemoteNodeDTO {
	result := make([]RemoteNodeDTO, len(ns))
	for i, n := range ns {
		result[i] = RemoteNodeToDTO(n)
	}
	return result
}

// KnownHostToDTO maps a domain known host entry to a DTO.
func KnownHostToDTO(e domain.KnownHostEntry) KnownHostDTO {
	return KnownHostDTO{
		Host: e.Host, KeyType: e.KeyType, Fingerprint: e.Fingerprint,
	}
}

// KnownHostsToDTO maps a slice of domain known host entries to DTOs.
func KnownHostsToDTO(es []domain.KnownHostEntry) []KnownHostDTO {
	result := make([]KnownHostDTO, len(es))
	for i, e := range es {
		result[i] = KnownHostToDTO(e)
	}
	return result
}

// DTOToFolder maps a FolderDTO back to a domain folder.
func DTOToFolder(d FolderDTO) domain.ConnectionFolder {
	return domain.ConnectionFolder{
		ID: d.ID, Name: d.Name, ParentID: d.ParentID, Order: d.Order,
	}
}

// DTOToConnection maps a ConnectionDTO back to a domain connection.
func DTOToConnection(d ConnectionDTO) domain.Connection {
	c := domain.Connection{
		ID:            d.ID,
		FolderID:      d.FolderID,
		Name:          d.Name,
		Host:          d.Host,
		Port:          d.Port,
		Order:         d.Order,
		User:          d.User,
		IdentityIDs:   d.IdentityIDs,
		Protocol:      d.Protocol,
		DefaultUserID: d.DefaultUserID,
		Tags:          d.Tags,
		VpnProfileID:  d.VpnProfileID,
	}
	for _, u := range d.Users {
		c.Users = append(c.Users, dtoToConnectionUser(u))
	}
	for _, h := range d.JumpChain {
		c.JumpChain.Hops = append(c.JumpChain.Hops, dtoToJumpHop(h))
	}
	if d.Proxy != nil && (d.Proxy.Host != "" || d.Proxy.Port != 0) {
		c.Proxy = &domain.ProxyConfig{
			Type:       d.Proxy.Type,
			Host:       d.Proxy.Host,
			Port:       d.Proxy.Port,
			Username:   d.Proxy.Username,
			PasswordID: d.Proxy.PasswordID,
		}
		if c.Proxy.Type == "" {
			c.Proxy.Type = "socks5"
		}
	}
	if d.TelnetConfig != nil {
		c.TelnetConfig = &domain.TelnetConfig{
			Host:       d.TelnetConfig.Host,
			Port:       d.TelnetConfig.Port,
			Username:   d.TelnetConfig.Username,
			PasswordID: d.TelnetConfig.PasswordID,
		}
		if c.TelnetConfig.Port == 0 {
			c.TelnetConfig.Port = 23
		}
	}
	if d.RDPConfig != nil {
		c.RDPConfig = &domain.RDPConfig{
			Host:       d.RDPConfig.Host,
			Port:       d.RDPConfig.Port,
			Username:   d.RDPConfig.Username,
			PasswordID: d.RDPConfig.PasswordID,
			Domain:     d.RDPConfig.Domain,
		}
		if c.RDPConfig.Port == 0 {
			c.RDPConfig.Port = 3389
		}
		// Sync credentials from default user when users exist (single source of truth)
		if du := c.DefaultUser(); du != nil {
			if du.Username != "" {
				c.RDPConfig.Username = du.Username
			}
			if du.PassAuth != nil && du.PassAuth.PasswordID != "" {
				c.RDPConfig.PasswordID = du.PassAuth.PasswordID
			}
		}
	}
	if d.SerialConfig != nil {
		c.SerialConfig = &domain.SerialConfig{
			Port:     d.SerialConfig.Port,
			BaudRate: d.SerialConfig.BaudRate,
			DataBits: d.SerialConfig.DataBits,
			StopBits: d.SerialConfig.StopBits,
			Parity:   d.SerialConfig.Parity,
		}
		if c.SerialConfig.BaudRate == 0 {
			c.SerialConfig.BaudRate = 9600
		}
		if c.SerialConfig.DataBits == 0 {
			c.SerialConfig.DataBits = 8
		}
		if c.SerialConfig.StopBits == 0 {
			c.SerialConfig.StopBits = 1
		}
		if c.SerialConfig.Parity == "" {
			c.SerialConfig.Parity = "none"
		}
	}
	if d.HTTPConfig != nil {
		c.HTTPConfig = &domain.HTTPConfig{
			URL:        d.HTTPConfig.URL,
			Method:     d.HTTPConfig.Method,
			Auth:       d.HTTPConfig.Auth,
			PasswordID: d.HTTPConfig.PasswordID,
		}
		if c.HTTPConfig.Method == "" {
			c.HTTPConfig.Method = "GET"
		}
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
	if d.KeyAuth != nil {
		u.KeyAuth = &domain.KeyAuthConfig{IdentityIDs: d.KeyAuth.IdentityIDs}
	}
	if d.PassAuth != nil {
		u.PassAuth = &domain.PasswordAuthConfig{PasswordID: d.PassAuth.PasswordID}
	}
	return u
}

func dtoToJumpHop(d JumpHopDTO) domain.JumpHop {
	h := domain.JumpHop{
		Host:     d.Host,
		Port:     d.Port,
		Username: d.Username,
		Auth:     domain.AuthMethodType(d.Auth),
	}
	if d.KeyAuth != nil {
		h.KeyAuth = &domain.KeyAuthConfig{IdentityIDs: d.KeyAuth.IdentityIDs}
	}
	if d.PassAuth != nil {
		h.PassAuth = &domain.PasswordAuthConfig{PasswordID: d.PassAuth.PasswordID}
	}
	return h
}

// VPNProfileDTO is the UI-facing representation of a VPN profile.
type VPNProfileDTO struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Protocol string `json:"protocol"`
}

// PuTTYSessionDTO is a preview item for REG import.
type PuTTYSessionDTO struct {
	Name     string `json:"name"`
	HostName string `json:"hostName"`
	Port     int    `json:"port"`
	UserName string `json:"userName"`
}

// LocalNodeDTO represents a local file or directory entry.
type LocalNodeDTO struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	ModTime string `json:"modTime,omitempty"`
	Mode    string `json:"mode,omitempty"`
	Owner   string `json:"owner,omitempty"`
}

// AppSettingsDTO is the UI-facing representation of application settings.
type AppSettingsDTO struct {
	LockoutEnabled           bool   `json:"lockoutEnabled"`
	LockoutIdleMinutes       int    `json:"lockoutIdleMinutes"`
	LockOnMinimize           bool   `json:"lockOnMinimize"`
	TerminalFontFamily       string `json:"terminalFontFamily"`
	TerminalFontSize         int    `json:"terminalFontSize"`
	TerminalFontColor        string `json:"terminalFontColor"`
	Theme                    string `json:"theme"`
	PingEnabled              bool   `json:"pingEnabled"`
	PingMode                 string `json:"pingMode"`
	PingIntervalSeconds      int    `json:"pingIntervalSeconds"`
	PingIntervalMin          int    `json:"pingIntervalMin"`
	ExternalEditorPath       string `json:"externalEditorPath"`
	TransferSpeedLimitKbps   int    `json:"transferSpeedLimitKbps"`
	ConnectionTimeoutSeconds int    `json:"connectionTimeoutSeconds"`
	MaxConcurrentTransfers   int    `json:"maxConcurrentTransfers"`
	SessionHotkeyCreate      string `json:"sessionHotkeyCreate"`
	SessionHotkeyNext        string `json:"sessionHotkeyNext"`
	SessionHotkeyPrev        string `json:"sessionHotkeyPrev"`
	SessionHotkeyClose       string `json:"sessionHotkeyClose"`
}

// AuditEntryDTO is the UI-facing representation of an audit log entry.
type AuditEntryDTO struct {
	ID           int64  `json:"id"`
	Timestamp    string `json:"timestamp"`
	SessionID    string `json:"sessionId"`
	ConnectionID string `json:"connectionId"`
	Username     string `json:"username"`
	Input        string `json:"input"`
	Redacted     bool   `json:"redacted"`
}
