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
	ID       string             `json:"id"`
	Host     string             `json:"host"`
	Port     int                `json:"port"`
	Username string             `json:"username"`
	Auth     string             `json:"authMethod"`
	KeyAuth  *KeyAuthConfigDTO  `json:"keyAuth,omitempty"`
	PassAuth *PassAuthConfigDTO `json:"passAuth,omitempty"`
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
	JumpChain     []JumpHopDTO        `json:"jumpChain,omitempty"`
}

// IdentityDTO is the UI-facing representation of an SSH identity.
type IdentityDTO struct {
	ID        string `json:"id"`
	Comment   string `json:"comment"`
	KeyType   string `json:"keyType"`
	Encrypted bool   `json:"encrypted"`
}

// KnownHostDTO is the UI-facing representation of a known host entry.
type KnownHostDTO struct {
	Host        string `json:"host"`
	KeyType     string `json:"keyType"`
	Fingerprint string `json:"fingerprint"`
}

// FolderToDTO maps a domain folder to a DTO.
func FolderToDTO(f domain.ConnectionFolder) FolderDTO {
	return FolderDTO{ID: f.ID, Name: f.Name, ParentID: f.ParentID, Order: f.Order}
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
		ID:       h.ID,
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
	return IdentityDTO{ID: id.ID, Comment: id.Comment, KeyType: id.KeyType, Encrypted: id.Encrypted}
}

// IdentitiesToDTO maps a slice of domain identities to DTOs.
func IdentitiesToDTO(ids []domain.SSHIdentity) []IdentityDTO {
	result := make([]IdentityDTO, len(ids))
	for i, id := range ids {
		result[i] = IdentityToDTO(id)
	}
	return result
}

// KnownHostToDTO maps a domain known host entry to a DTO.
func KnownHostToDTO(e domain.KnownHostEntry) KnownHostDTO {
	return KnownHostDTO{Host: e.Host, KeyType: e.KeyType, Fingerprint: e.Fingerprint}
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
	return domain.ConnectionFolder{ID: d.ID, Name: d.Name, ParentID: d.ParentID, Order: d.Order}
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
	}
	for _, u := range d.Users {
		c.Users = append(c.Users, dtoToConnectionUser(u))
	}
	for _, h := range d.JumpChain {
		c.JumpChain.Hops = append(c.JumpChain.Hops, dtoToJumpHop(h))
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
		ID:       d.ID,
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
