package usecase

import (
	"encoding/base64"
	"fmt"

	domainplugin "xquakshell/internal/domain/plugin"
)

// errSurfaceConsumerBehind is what a plugin gets when its surface's queue stayed full for the
// whole write allowance. It is ErrRateLimited, which the IPC layer answers as -32003 — the same
// backpressure verdict session.writeTerminal gives, because it is the same situation.
var errSurfaceConsumerBehind = fmt.Errorf("%w: surface consumer is behind", domainplugin.ErrRateLimited)

// surfaceStreamOrder fixes the order streams are flushed in, so a batch containing both is emitted
// the same way every time. stdout first, because that is the one a reader is usually following.
var surfaceStreamOrder = [...]string{surfaceStreamStdout, surfaceStreamStderr}

// surfaceBatch accumulates one flush interval's output, keeping the streams apart.
//
// Separate buffers rather than one, because the two streams mean different things to the viewer:
// a log surface colours them apart, and concatenating them would splice a half-written stdout line
// into a stderr one.
type surfaceBatch struct {
	buffers map[string][]byte
}

func newSurfaceBatch() *surfaceBatch {
	return &surfaceBatch{buffers: make(map[string][]byte, len(surfaceStreamOrder))}
}

func (b *surfaceBatch) add(chunk surfaceChunk) {
	if len(chunk.data) == 0 {
		return
	}
	b.buffers[chunk.stream] = append(b.buffers[chunk.stream], chunk.data...)
}

// take removes and returns one stream's accumulated bytes.
func (b *surfaceBatch) take(stream string) []byte {
	data := b.buffers[stream]
	if len(data) == 0 {
		return nil
	}
	delete(b.buffers, stream)
	return data
}

// encodeSurfaceOutput renders bytes for the frontend, which decodes base64 for both producers.
func encodeSurfaceOutput(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// decodeSurfaceOutput reads what a plugin sent on surface.write.
//
// Decoding here rather than passing the string through is what makes an invalid payload a refusal
// instead of garbage on screen: the frontend's decoder falls back to treating an undecodable
// string as raw bytes, so a malformed write used to arrive as the literal base64 text.
func decodeSurfaceOutput(dataBase64 string) ([]byte, error) {
	if dataBase64 == "" {
		return nil, nil
	}
	data, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		return nil, fmt.Errorf("surface: dataBase64 is not valid base64")
	}
	return data, nil
}
