package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"xquakshell/internal/domain"
)

const (
	// MaxReplicaBytes bounds a sealed replica in either direction.
	//
	// The transport is assumed hostile, and without a ceiling a plugin announces a replica of any
	// size and the host allocates for it. Eight mebibytes is far more than a scope of connections
	// and their secrets can occupy, and half of what chunked plugin I/O already reaches elsewhere
	// (MaxFileBytes). The same ceiling applies outbound, so a scope that has outgrown it fails
	// loudly here rather than at whatever limit the plugin's own server happens to have - where the
	// user would see it as an opaque failure from a third party.
	MaxReplicaBytes = 8 << 20

	// ReplicaChunkBytes is how much of a replica moves in one RPC.
	//
	// A replica cannot travel in a single call: every JSON-RPC frame is capped at MaxFrameBytes,
	// and a scope holding a handful of SSH keys already exceeds that. So it is chunked, the same
	// way fs.read and fs.write already chunk a file across frames. The size leaves room for base64
	// inflating it by a third and for the JSON envelope around it, which is why it is a fraction of
	// the frame rather than equal to it.
	ReplicaChunkBytes = 128 << 10
)

// replicaCaller is the slice of the plugin RPC surface this transport needs. Narrow on purpose: the
// wire shape is worth testing without a plugin process behind it.
type replicaCaller interface {
	Call(ctx context.Context, pluginID, method string, params json.RawMessage) (json.RawMessage, error)
}

// PluginReplicaTransport moves sealed bytes through a plugin (ADR-022, transport A).
//
// It knows nothing about what it is moving, which is the point: the document is sealed before it
// reaches here and opened after it comes back, so a hostile plugin and a hostile server both handle
// ciphertext only. What this type owns is everything a plugin must not be trusted with - how much
// it may hand back, whether the version it is serving stayed still, and the difference between "the
// remote is empty" and "the call failed".
type PluginReplicaTransport struct {
	caller replicaCaller
}

// NewPluginReplicaTransport creates the transport the replication use case moves bytes through.
func NewPluginReplicaTransport(caller replicaCaller) *PluginReplicaTransport {
	return &PluginReplicaTransport{caller: caller}
}

// replicaFetchParams asks for the bytes from offset onwards. The host drives the offsets rather
// than counting on a fixed chunk size, so a remote is free to answer with less than it was asked
// for - which is what any real HTTP or file-backed implementation will end up doing.
type replicaFetchParams struct {
	Offset int `json:"offset"`
}

type replicaFetchResult struct {
	Token       string `json:"token"`
	TotalBytes  int    `json:"totalBytes"`
	ChunkBase64 string `json:"chunkBase64"`
}

// Fetch returns whatever the remote holds, and the token that names that version.
//
// A remote with nothing stored yet answers with neither, which is not an error: it is what the
// first device to synchronise sees. Every other outcome is an error rather than an empty answer,
// because empty is what the merge would read as the other device having deleted everything.
func (t *PluginReplicaTransport) Fetch(ctx context.Context, pluginID string) ([]byte, string, error) {
	if t == nil || t.caller == nil {
		return nil, "", errors.New("replica fetch: no plugin transport is wired")
	}
	first, err := t.readChunk(ctx, pluginID, 0)
	if err != nil {
		return nil, "", err
	}
	if first.TotalBytes == 0 && first.Token == "" {
		return nil, "", nil
	}
	if err := validateFetchHeader(pluginID, first); err != nil {
		return nil, "", err
	}
	sealed := make([]byte, 0, first.TotalBytes)
	for next := first; ; {
		sealed, err = appendChunk(pluginID, sealed, next)
		if err != nil {
			return nil, "", err
		}
		if len(sealed) == first.TotalBytes {
			return sealed, first.Token, nil
		}
		if next, err = t.readChunk(ctx, pluginID, len(sealed)); err != nil {
			return nil, "", err
		}
		if next.Token != first.Token || next.TotalBytes != first.TotalBytes {
			return nil, "", fmt.Errorf("replica fetch from %s: the version moved mid-read: %w",
				pluginID, domain.ErrReplicaOpenFailed)
		}
	}
}

// readChunk makes one fetch call. Cancellation is checked before it rather than only inside, so a
// window closing mid-sync stops the loop instead of running it to completion against a remote that
// keeps answering.
func (t *PluginReplicaTransport) readChunk(ctx context.Context, pluginID string, offset int) (replicaFetchResult, error) {
	if err := ctx.Err(); err != nil {
		return replicaFetchResult{}, fmt.Errorf("replica fetch from %s: %w", pluginID, err)
	}
	params, err := json.Marshal(replicaFetchParams{Offset: offset})
	if err != nil {
		return replicaFetchResult{}, fmt.Errorf("replica fetch from %s: %w", pluginID, err)
	}
	raw, err := t.caller.Call(ctx, pluginID, "replica.fetch", params)
	if err != nil {
		return replicaFetchResult{}, fmt.Errorf("replica fetch from %s: %w", pluginID, err)
	}
	var result replicaFetchResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return replicaFetchResult{}, fmt.Errorf("replica fetch from %s: %w", pluginID, err)
	}
	return result, nil
}

// validateFetchHeader checks what the first answer declares, before anything is allocated for it.
func validateFetchHeader(pluginID string, first replicaFetchResult) error {
	switch {
	case first.TotalBytes < 0 || first.TotalBytes > MaxReplicaBytes:
		return fmt.Errorf("replica fetch from %s: declared %d bytes, ceiling is %d: %w",
			pluginID, first.TotalBytes, MaxReplicaBytes, domain.ErrReplicaOpenFailed)
	case first.Token == "":
		// Bytes with no version cannot be pushed against later: the next push would have nothing
		// to compare and swap on, making it a blind overwrite of whatever arrived in between.
		return fmt.Errorf("replica fetch from %s: bytes served without a version token: %w",
			pluginID, domain.ErrReplicaOpenFailed)
	}
	return nil
}

// appendChunk decodes one chunk onto what has been read so far.
//
// The three refusals are what stops a declared-size check being bypassed one chunk at a time: a
// chunk that overruns the announced total, one larger than a frame could have carried, and one that
// is empty before the end - which a trusting loop would spin on forever.
func appendChunk(pluginID string, sealed []byte, next replicaFetchResult) ([]byte, error) {
	chunk, err := base64.StdEncoding.DecodeString(next.ChunkBase64)
	if err != nil {
		return nil, fmt.Errorf("replica fetch from %s: %w", pluginID, err)
	}
	switch {
	case len(chunk) == 0:
		return nil, fmt.Errorf("replica fetch from %s: empty chunk at offset %d of %d: %w",
			pluginID, len(sealed), next.TotalBytes, domain.ErrReplicaOpenFailed)
	case len(chunk) > ReplicaChunkBytes:
		return nil, fmt.Errorf("replica fetch from %s: chunk of %d bytes exceeds %d: %w",
			pluginID, len(chunk), ReplicaChunkBytes, domain.ErrReplicaOpenFailed)
	case len(sealed)+len(chunk) > next.TotalBytes:
		return nil, fmt.Errorf("replica fetch from %s: chunk overruns the declared %d bytes: %w",
			pluginID, next.TotalBytes, domain.ErrReplicaOpenFailed)
	}
	return append(sealed, chunk...), nil
}
