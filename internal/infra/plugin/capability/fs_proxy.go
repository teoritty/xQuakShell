package capability

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/portable"
	"ssh-client/internal/pkg/pathsafe"
)

// MaxReadBytes re-exports the domain limit for tests and infra.
const MaxReadBytes = domainplugin.MaxReadBytes

// FSProxy serves sandboxed filesystem RPC for a plugin.
type FSProxy struct {
	pluginDataDir string
	readRoots     []string
	writeRoots    []string
}

// NewFSProxy creates an FS proxy from manifest FS capabilities.
func NewFSProxy(caps *domainplugin.FSCaps, pluginDataDir string) (*FSProxy, error) {
	var readPatterns, writePatterns []string
	if caps != nil {
		readPatterns = caps.Read
		writePatterns = caps.Write
	}
	readRoots, err := resolveRoots(readPatterns, pluginDataDir)
	if err != nil {
		return nil, err
	}
	writeRoots, err := resolveRoots(writePatterns, pluginDataDir)
	if err != nil {
		return nil, err
	}
	return &FSProxy{
		pluginDataDir: pluginDataDir,
		readRoots:     readRoots,
		writeRoots:    writeRoots,
	}, nil
}

type fsPathParams struct {
	Path string `json:"path"`
}

type fsReadParams struct {
	Path     string `json:"path"`
	Offset   int64  `json:"offset,omitempty"`
	MaxBytes int    `json:"maxBytes,omitempty"`
}

type fsReadResult struct {
	ContentBase64 string `json:"contentBase64"`
	Offset        int64  `json:"offset"`
	TotalSize     int64  `json:"totalSize"`
	EOF           bool   `json:"eof"`
}

type fsWriteParams struct {
	Path          string `json:"path"`
	ContentBase64 string `json:"contentBase64"`
	Offset        *int64 `json:"offset,omitempty"`
}

type fsListResult struct {
	Entries []fsEntry `json:"entries"`
}

type fsEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
}

// Handle dispatches fs.* RPC methods.
func (p *FSProxy) Handle(method string, params json.RawMessage) (json.RawMessage, error) {
	switch method {
	case "fs.read":
		return p.read(params)
	case "fs.write":
		return p.write(params)
	case "fs.list":
		return p.list(params)
	default:
		return nil, domainplugin.ErrCapabilityDenied
	}
}

func (p *FSProxy) resolveSafePath(requestPath string, write bool) (string, error) {
	path, err := ResolvePath(requestPath, p.pluginDataDir, p.readRoots, p.writeRoots, write)
	if err != nil {
		return "", err
	}
	roots := p.readRoots
	if write {
		roots = p.writeRoots
	}
	return SecurePathUnderRoots(path, roots)
}

func (p *FSProxy) read(params json.RawMessage) (json.RawMessage, error) {
	var req fsReadParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid fs.read params: %w", err)
	}
	path, err := p.resolveSafePath(req.Path, false)
	if err != nil {
		return nil, fmt.Errorf("fs.read: %w", err)
	}
	maxBytes := int64(req.MaxBytes)
	if maxBytes <= 0 || maxBytes > domainplugin.MaxReadBytes {
		maxBytes = domainplugin.MaxReadBytes
	}
	if req.Offset < 0 {
		return nil, fmt.Errorf("invalid fs.read params: negative offset")
	}

	chunk, err := pathsafe.ReadFileChunk(p.readRoots, path, req.Offset, maxBytes)
	if err != nil {
		if err == pathsafe.ErrPathDenied {
			return nil, domainplugin.ErrCapabilityDenied
		}
		return nil, fmt.Errorf("fs.read: %w", err)
	}
	if chunk.TotalSize > domainplugin.MaxFileBytes {
		return nil, domainplugin.ErrCapabilityDenied
	}

	result, err := json.Marshal(fsReadResult{
		ContentBase64: base64.StdEncoding.EncodeToString(chunk.Data),
		Offset:        chunk.Offset,
		TotalSize:     chunk.TotalSize,
		EOF:           chunk.EOF,
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (p *FSProxy) write(params json.RawMessage) (json.RawMessage, error) {
	var req fsWriteParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid fs.write params: %w", err)
	}
	path, err := p.resolveSafePath(req.Path, true)
	if err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(req.ContentBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid contentBase64: %w", err)
	}
	if len(data) > domainplugin.MaxWriteBytes {
		return nil, domainplugin.ErrCapabilityDenied
	}
	if err := portable.RequireWritable(); err != nil {
		return nil, err
	}

	var resultingSize int64
	if req.Offset == nil {
		resultingSize = int64(len(data))
	} else {
		if *req.Offset < 0 {
			return nil, fmt.Errorf("invalid fs.write params: negative offset")
		}
		resultingSize = *req.Offset + int64(len(data))
	}
	if resultingSize > domainplugin.MaxFileBytes {
		return nil, domainplugin.ErrCapabilityDenied
	}

	if req.Offset == nil {
		if err := pathsafe.WriteExistingFile(p.writeRoots, path, data, 0o600); err != nil {
			if err == pathsafe.ErrPathDenied {
				return nil, domainplugin.ErrCapabilityDenied
			}
			return nil, fmt.Errorf("fs.write: %w", err)
		}
	} else {
		if err := pathsafe.WriteFileChunk(p.writeRoots, path, *req.Offset, data, 0o600); err != nil {
			if err == pathsafe.ErrPathDenied {
				return nil, domainplugin.ErrCapabilityDenied
			}
			return nil, fmt.Errorf("fs.write: %w", err)
		}
	}
	return json.Marshal(map[string]bool{"ok": true})
}

func (p *FSProxy) list(params json.RawMessage) (json.RawMessage, error) {
	var req fsPathParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid fs.list params: %w", err)
	}
	path, err := p.resolveSafePath(req.Path, false)
	if err != nil {
		return nil, fmt.Errorf("fs.list: %w", err)
	}
	dir, err := pathsafe.OpenExistingDir(p.readRoots, path)
	if err != nil {
		if err == pathsafe.ErrPathDenied {
			return nil, domainplugin.ErrCapabilityDenied
		}
		return nil, fmt.Errorf("fs.list: %w", err)
	}
	defer dir.Close()

	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, fmt.Errorf("fs.list: %w", err)
	}
	out := make([]fsEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, fsEntry{Name: e.Name(), IsDir: e.IsDir()})
	}
	return json.Marshal(fsListResult{Entries: out})
}
