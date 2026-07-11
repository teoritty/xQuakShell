package ipc

import (
	"encoding/binary"

	domainplugin "ssh-client/internal/domain/plugin"
)

// FrameHeader is the fixed 9-byte frame header: 4B length + 1B kind + 4B channelId.
type FrameHeader struct {
	Length    uint32
	Kind      byte
	ChannelID uint32
}

func encodeHeader(h FrameHeader) [domainplugin.FrameHeaderLen]byte {
	var b [domainplugin.FrameHeaderLen]byte
	binary.BigEndian.PutUint32(b[0:4], h.Length)
	b[4] = h.Kind
	binary.BigEndian.PutUint32(b[5:9], h.ChannelID)
	return b
}

func decodeHeader(b []byte) FrameHeader {
	return FrameHeader{
		Length:    binary.BigEndian.Uint32(b[0:4]),
		Kind:      b[4],
		ChannelID: binary.BigEndian.Uint32(b[5:9]),
	}
}
