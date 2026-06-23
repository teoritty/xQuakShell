package pluginsdk

import "encoding/base64"

// FSReadParams reads bytes from the plugin sandbox.
type FSReadParams struct {
	Path     string `json:"path"`
	Offset   int64  `json:"offset,omitempty"`
	MaxBytes int    `json:"maxBytes,omitempty"`
}

// FSReadResult is the fs.read response payload.
type FSReadResult struct {
	ContentBase64 string `json:"contentBase64"`
	Offset        int64  `json:"offset"`
	TotalSize     int64  `json:"totalSize"`
	EOF           bool   `json:"eof"`
}

// FSWriteParams writes bytes into the plugin sandbox.
type FSWriteParams struct {
	Path          string `json:"path"`
	ContentBase64 string `json:"contentBase64"`
	Offset        *int64 `json:"offset,omitempty"`
}

// ReadFile reads an entire sandboxed file via chunked fs.read calls.
func (c *Client) ReadFile(path string) ([]byte, error) {
	var out []byte
	var offset int64
	for {
		var chunk FSReadResult
		if err := c.CallCore("fs.read", FSReadParams{
			Path:     path,
			Offset:   offset,
		}, &chunk); err != nil {
			return nil, err
		}
		if chunk.ContentBase64 != "" {
			data, err := base64.StdEncoding.DecodeString(chunk.ContentBase64)
			if err != nil {
				return nil, err
			}
			out = append(out, data...)
			offset = chunk.Offset + int64(len(data))
		}
		if chunk.EOF {
			if len(out) == 0 {
				return nil, nil
			}
			return out, nil
		}
	}
}

// ReadFileChunk reads one fs.read chunk from the sandbox.
func (c *Client) ReadFileChunk(path string, offset int64, maxBytes int) (FSReadResult, error) {
	var out FSReadResult
	err := c.CallCore("fs.read", FSReadParams{
		Path:     path,
		Offset:   offset,
		MaxBytes: maxBytes,
	}, &out)
	return out, err
}

// WriteFile replaces a sandboxed file via fs.write.
func (c *Client) WriteFile(path string, data []byte) error {
	return c.CallCore("fs.write", FSWriteParams{
		Path:          path,
		ContentBase64: base64.StdEncoding.EncodeToString(data),
	}, nil)
}

// WriteFileChunk writes bytes at offset via fs.write.
func (c *Client) WriteFileChunk(path string, offset int64, data []byte) error {
	off := offset
	return c.CallCore("fs.write", FSWriteParams{
		Path:          path,
		ContentBase64: base64.StdEncoding.EncodeToString(data),
		Offset:        &off,
	}, nil)
}
