package pluginsdk

import "encoding/base64"

// NetDialParams requests an outbound connection via core allowlist.
type NetDialParams struct {
	Network string `json:"network"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
}

// NetDialResult is the net.dial response payload.
type NetDialResult struct {
	HandleID string `json:"handleId"`
}

// NetHandleParams identifies a core-owned network handle.
type NetHandleParams struct {
	HandleID string `json:"handleId"`
}

// NetReadParams reads bytes from a network handle.
type NetReadParams struct {
	HandleID string `json:"handleId"`
	MaxBytes int    `json:"maxBytes,omitempty"`
}

// NetReadResult is the net.read response payload.
type NetReadResult struct {
	ContentBase64 string `json:"contentBase64"`
	EOF           bool   `json:"eof"`
}

// NetWriteParams writes bytes to a network handle.
type NetWriteParams struct {
	HandleID      string `json:"handleId"`
	ContentBase64 string `json:"contentBase64"`
}

// NetClient proxies outbound network I/O through the core capability gate.
type NetClient struct {
	client *Client
}

// Net returns a typed network client backed by core net.* RPC methods.
func (c *Client) Net() *NetClient {
	return &NetClient{client: c}
}

// Dial opens an allowlisted TCP connection and returns a core-owned handle ID.
func (n *NetClient) Dial(host string, port int) (string, error) {
	if n == nil || n.client == nil {
		return "", ErrClientUnavailable()
	}
	var out NetDialResult
	err := n.client.CallCore("net.dial", NetDialParams{
		Network: "tcp",
		Host:    host,
		Port:    port,
	}, &out)
	if err != nil {
		return "", err
	}
	return out.HandleID, nil
}

// Read decodes bytes from a handle via net.read.
func (n *NetClient) Read(handleID string, maxBytes int) ([]byte, bool, error) {
	if n == nil || n.client == nil {
		return nil, false, ErrClientUnavailable()
	}
	var out NetReadResult
	if err := n.client.CallCore("net.read", NetReadParams{
		HandleID: handleID,
		MaxBytes: maxBytes,
	}, &out); err != nil {
		return nil, false, err
	}
	if out.ContentBase64 == "" {
		return nil, out.EOF, nil
	}
	data, err := base64.StdEncoding.DecodeString(out.ContentBase64)
	if err != nil {
		return nil, false, err
	}
	return data, out.EOF, nil
}

// Write sends bytes to a handle via net.write.
func (n *NetClient) Write(handleID string, data []byte) error {
	if n == nil || n.client == nil {
		return ErrClientUnavailable()
	}
	return n.client.CallCore("net.write", NetWriteParams{
		HandleID:      handleID,
		ContentBase64: base64.StdEncoding.EncodeToString(data),
	}, nil)
}

// Close releases a handle via net.close.
func (n *NetClient) Close(handleID string) error {
	if n == nil || n.client == nil {
		return ErrClientUnavailable()
	}
	return n.client.CallCore("net.close", NetHandleParams{HandleID: handleID}, nil)
}

// ErrClientUnavailable reports a nil pluginsdk client.
func ErrClientUnavailable() error {
	return &CoreError{Code: -32603, Message: "pluginsdk client unavailable"}
}
