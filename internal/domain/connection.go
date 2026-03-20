package domain

import "fmt"

const (
	// DefaultSSHPort is the standard SSH port.
	DefaultSSHPort = 22
	// MinPort is the minimum valid TCP port number.
	MinPort = 1
	// MaxPort is the maximum valid TCP port number.
	MaxPort = 65535
)

// ProxyConfig configures a SOCKS proxy for outbound connections.
type ProxyConfig struct {
	Type       string `json:"type"`        // "socks5" or "socks4"
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username,omitempty"`
	PasswordID string `json:"passwordId,omitempty"`
}

// IsEmpty returns true if the proxy is not configured.
func (p *ProxyConfig) IsEmpty() bool {
	return p == nil || (p.Host == "" && p.Port == 0)
}

// ProtocolType identifies the connection protocol.
const (
	ProtocolSSH    = "ssh"
	ProtocolRDP   = "rdp"
	ProtocolTelnet = "telnet"
	ProtocolSerial = "serial"
	ProtocolHTTP   = "http"
)

// TelnetConfig holds Telnet-specific connection parameters.
type TelnetConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username,omitempty"`
	PasswordID string `json:"passwordId,omitempty"`
}

// RDPConfig holds RDP-specific connection parameters.
type RDPConfig struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username,omitempty"`
	PasswordID string `json:"passwordId,omitempty"`
	Domain     string `json:"domain,omitempty"`
}

// SerialConfig holds serial port connection parameters.
type SerialConfig struct {
	Port     string `json:"port"`     // COM1, /dev/ttyUSB0
	BaudRate int    `json:"baudRate"`
	DataBits int    `json:"dataBits"`
	StopBits int    `json:"stopBits"`
	Parity   string `json:"parity"`
}

// HTTPConfig holds HTTP/HTTPS connection parameters.
type HTTPConfig struct {
	URL     string `json:"url"`
	Method  string `json:"method"`
	Auth    string `json:"auth,omitempty"` // basic, bearer
	PasswordID string `json:"passwordId,omitempty"`
}

// Connection holds the configuration for a connection.
// Users holds one or more ConnectionUser references; DefaultUserID selects the active one.
// Legacy fields User and IdentityIDs are kept for vault v1 backward compatibility
// and are migrated into Users on vault upgrade.
type Connection struct {
	ID          string   `json:"id"`
	FolderID    string   `json:"folderId"`
	Name        string   `json:"name"`
	Host        string   `json:"host"`
	Port        int      `json:"port"`
	Order       int      `json:"order"`

	// Legacy (v1) — migrated to Users on vault v2 upgrade.
	User        string   `json:"user,omitempty"`
	IdentityIDs []string `json:"identityIds,omitempty"`

	// v2 fields
	Protocol      string           `json:"protocol,omitempty"` // ssh, rdp, telnet, serial, http
	Users         []ConnectionUser `json:"users,omitempty"`
	DefaultUserID string           `json:"defaultUserId,omitempty"`
	Tags          []string         `json:"tags,omitempty"`
	VpnProfileID  string           `json:"vpnProfileId,omitempty"`
	JumpChain     JumpChainConfig  `json:"jumpChain,omitempty"`
	Proxy         *ProxyConfig     `json:"proxy,omitempty"`

	// Protocol-specific configs (used when Protocol != ssh)
	TelnetConfig *TelnetConfig `json:"telnetConfig,omitempty"`
	RDPConfig    *RDPConfig    `json:"rdpConfig,omitempty"`
	SerialConfig *SerialConfig `json:"serialConfig,omitempty"`
	HTTPConfig   *HTTPConfig  `json:"httpConfig,omitempty"`
}

// DefaultUser returns the ConnectionUser designated as default, or nil when none is set.
func (c *Connection) DefaultUser() *ConnectionUser {
	for i := range c.Users {
		if c.Users[i].ID == c.DefaultUserID {
			return &c.Users[i]
		}
	}
	return nil
}

// EffectiveUsername returns the username that should be used when opening a session.
// It prefers the default user; falls back to the legacy User field.
func (c *Connection) EffectiveUsername() string {
	if u := c.DefaultUser(); u != nil {
		return u.Username
	}
	return c.User
}

// GetProtocol returns the connection protocol, defaulting to ssh.
func (c *Connection) GetProtocol() string {
	if c.Protocol != "" {
		return c.Protocol
	}
	return ProtocolSSH
}

// EffectiveHost returns the host to use for ping/connect based on protocol.
// SSH uses Connection.Host; RDP/Telnet use their config; Serial/HTTP return empty.
func (c *Connection) EffectiveHost() string {
	switch c.GetProtocol() {
	case ProtocolSSH:
		return c.Host
	case ProtocolRDP:
		if c.RDPConfig != nil {
			return c.RDPConfig.Host
		}
		return ""
	case ProtocolTelnet:
		if c.TelnetConfig != nil {
			return c.TelnetConfig.Host
		}
		return ""
	default:
		return ""
	}
}

// EffectivePort returns the port to use for ping/connect based on protocol.
func (c *Connection) EffectivePort() int {
	switch c.GetProtocol() {
	case ProtocolSSH:
		if c.Port > 0 {
			return c.Port
		}
		return DefaultSSHPort
	case ProtocolRDP:
		if c.RDPConfig != nil && c.RDPConfig.Port > 0 {
			return c.RDPConfig.Port
		}
		return 3389
	case ProtocolTelnet:
		if c.TelnetConfig != nil && c.TelnetConfig.Port > 0 {
			return c.TelnetConfig.Port
		}
		return 23
	default:
		return 0
	}
}

// Validate checks structural constraints (port range, jump hops, proxy).
// It deliberately allows empty host/username so draft connections can be saved.
// Full readiness should be checked via ValidateForConnect before opening a session.
func (c *Connection) Validate() error {
	if c.Port < MinPort || c.Port > MaxPort {
		return fmt.Errorf("port %d out of range [%d-%d]: %w", c.Port, MinPort, MaxPort, ErrInvalidConnectionConfig)
	}
	for i := range c.JumpChain.Hops {
		if err := c.JumpChain.Hops[i].Validate(); err != nil {
			return fmt.Errorf("jump hop %d: %w", i, err)
		}
	}
	if c.Proxy != nil && !c.Proxy.IsEmpty() && (c.Proxy.Port < MinPort || c.Proxy.Port > MaxPort) {
		return fmt.Errorf("proxy port %d out of range: %w", c.Proxy.Port, ErrInvalidConnectionConfig)
	}
	return nil
}

// ValidateForConnect performs strict validation required before opening a session.
func (c *Connection) ValidateForConnect() error {
	proto := c.GetProtocol()
	switch proto {
	case ProtocolSSH:
		if c.Host == "" {
			return fmt.Errorf("host must not be empty: %w", ErrInvalidConnectionConfig)
		}
		if c.EffectiveUsername() == "" {
			return fmt.Errorf("at least one user must be configured: %w", ErrInvalidConnectionConfig)
		}
	case ProtocolTelnet:
		if c.TelnetConfig == nil || c.TelnetConfig.Host == "" {
			return fmt.Errorf("telnet host must not be empty: %w", ErrInvalidConnectionConfig)
		}
	case ProtocolRDP:
		if c.RDPConfig == nil || c.RDPConfig.Host == "" {
			return fmt.Errorf("rdp host must not be empty: %w", ErrInvalidConnectionConfig)
		}
		if c.EffectiveUsername() == "" {
			return fmt.Errorf("at least one user must be configured for RDP: %w", ErrInvalidConnectionConfig)
		}
	case ProtocolSerial:
		if c.SerialConfig == nil || c.SerialConfig.Port == "" {
			return fmt.Errorf("serial port must not be empty: %w", ErrInvalidConnectionConfig)
		}
	case ProtocolHTTP:
		if c.HTTPConfig == nil || c.HTTPConfig.URL == "" {
			return fmt.Errorf("http url must not be empty: %w", ErrInvalidConnectionConfig)
		}
	default:
		return fmt.Errorf("unknown protocol %q: %w", proto, ErrInvalidConnectionConfig)
	}
	return c.Validate()
}

// WithDefaults fills in default values for optional fields (e.g., port).
func (c *Connection) WithDefaults() {
	if c.Port == 0 {
		c.Port = DefaultSSHPort
	}
}
