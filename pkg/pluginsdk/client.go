package pluginsdk

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Client wraps Host with typed helpers for core RPC methods.
type Client struct {
	host    *Host
	timeout time.Duration
}

// NewClient creates a typed SDK client over a plugin Host.
func NewClient(host *Host) *Client {
	return &Client{host: host, timeout: DefaultCallTimeout}
}

// WithTimeout sets the default timeout for core RPC calls.
func (c *Client) WithTimeout(d time.Duration) *Client {
	c.timeout = d
	return c
}

// CallCore invokes a core JSON-RPC method and unmarshals the result.
func (c *Client) CallCore(method string, params any, result any) error {
	if c == nil || c.host == nil {
		return fmt.Errorf("pluginsdk client unavailable")
	}
	timeout := c.timeout
	if timeout <= 0 {
		timeout = DefaultCallTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	raw, err := c.host.CallCoreContext(ctx, method, params)
	if err != nil {
		return err
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(raw, result)
}

// Ping verifies connectivity to the core host.
func (c *Client) Ping() (map[string]string, error) {
	var out map[string]string
	if err := c.CallCore("ping", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
