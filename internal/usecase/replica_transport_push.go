package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"xquakshell/internal/domain"
)

// replicaPushParams carries one chunk and, on every call, the version the whole replica was built
// from. Repeating it lets the remote refuse at any point in the upload rather than only at the
// start, which matters because another device can write while this one is still sending.
type replicaPushParams struct {
	ExpectedToken string `json:"expectedToken"`
	Offset        int    `json:"offset"`
	TotalBytes    int    `json:"totalBytes"`
	ChunkBase64   string `json:"chunkBase64"`
}

type replicaPushResult struct {
	Token    string `json:"token"`
	Conflict bool   `json:"conflict"`
}

// Push stores the bytes if the remote still holds the version they were built from.
//
// A refusal comes back as ErrReplicaConflict, which is not a failure to report to the user: another
// device wrote first, and the answer is to read again and merge.
func (t *PluginReplicaTransport) Push(ctx context.Context, pluginID string, sealed []byte, expected string) (string, error) {
	if t == nil || t.caller == nil {
		return "", errors.New("replica push: no plugin transport is wired")
	}
	if err := validatePushSize(pluginID, sealed); err != nil {
		return "", err
	}
	var last replicaPushResult
	for offset := 0; offset < len(sealed); {
		end := min(offset+ReplicaChunkBytes, len(sealed))
		result, err := t.writeChunk(ctx, pluginID, replicaPushParams{
			ExpectedToken: expected,
			Offset:        offset,
			TotalBytes:    len(sealed),
			ChunkBase64:   base64.StdEncoding.EncodeToString(sealed[offset:end]),
		})
		if err != nil {
			return "", err
		}
		if result.Conflict {
			return "", fmt.Errorf("replica push to %s: %w", pluginID, domain.ErrReplicaConflict)
		}
		last, offset = result, end
	}
	// An accepted push with no token stored the bytes under a version nobody can name, so the next
	// push has nothing to compare and swap against. Carrying an empty token forward would turn every
	// subsequent push into a blind overwrite.
	if last.Token == "" {
		return "", fmt.Errorf("replica push to %s: accepted without a version token", pluginID)
	}
	return last.Token, nil
}

// validatePushSize refuses what must never reach the plugin at all.
func validatePushSize(pluginID string, sealed []byte) error {
	switch {
	case len(sealed) == 0:
		// Pushing nothing is how a wiped server would ask a healthy device to erase itself on the
		// next fetch. Deletion never travels (I9), so an empty replica is not something this
		// transport can be made to send.
		return fmt.Errorf("replica push to %s: %w", pluginID, domain.ErrReplicaEmpty)
	case len(sealed) > MaxReplicaBytes:
		return fmt.Errorf("replica push to %s: %d bytes exceeds the ceiling of %d",
			pluginID, len(sealed), MaxReplicaBytes)
	}
	return nil
}

// writeChunk sends one chunk. Cancellation is checked before it rather than only inside, so a window
// closing mid-sync stops the upload instead of running it to completion.
func (t *PluginReplicaTransport) writeChunk(ctx context.Context, pluginID string, chunk replicaPushParams) (replicaPushResult, error) {
	if err := ctx.Err(); err != nil {
		return replicaPushResult{}, fmt.Errorf("replica push to %s: %w", pluginID, err)
	}
	params, err := json.Marshal(chunk)
	if err != nil {
		return replicaPushResult{}, fmt.Errorf("replica push to %s: %w", pluginID, err)
	}
	raw, err := t.caller.Call(ctx, pluginID, "replica.push", params)
	if err != nil {
		return replicaPushResult{}, fmt.Errorf("replica push to %s: %w", pluginID, err)
	}
	var result replicaPushResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return replicaPushResult{}, fmt.Errorf("replica push to %s: %w", pluginID, err)
	}
	return result, nil
}
