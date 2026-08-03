package ipc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	domainplugin "xquakshell/internal/domain/plugin"
)

const jsonRPCVersion = "2.0"

// ErrParseError indicates a JSON-RPC frame could not be parsed.
var ErrParseError = errors.New("jsonrpc parse error")

// NewParseErrorResponse builds a JSON-RPC parse error response (id null).
func NewParseErrorResponse() Message {
	return Message{JSONRPC: jsonRPCVersion, Error: &RPCError{Code: -32700, Message: "Parse error"}}
}

// IsParseError reports whether err is a JSON-RPC parse failure.
func IsParseError(err error) bool {
	return errors.Is(err, ErrParseError)
}

// ErrFrameTooLarge indicates an RPC frame exceeded MaxFrameBytes.
var ErrFrameTooLarge = fmt.Errorf("rpc frame exceeds %d bytes", domainplugin.MaxFrameBytes)

// Message is a JSON-RPC 2.0 frame (request, response, or notification).
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *RPCID          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCID is a JSON-RPC 2.0 request id. The spec (§4, §5) permits the id to be a
// string or a number, and requires a response to echo the request's id back
// unchanged — so the host must not assume a single type. RPCID stores the id as
// its verbatim JSON token, which lets a response mirror any plugin's id
// byte-for-byte (numeric like `7`, or string like `"xqs-vnc-1"`) and makes
// equality a comparison of that canonical token via Key.
type RPCID struct {
	raw json.RawMessage
}

// MarshalJSON emits the id's verbatim JSON token, so responses echo it exactly.
func (id RPCID) MarshalJSON() ([]byte, error) {
	if len(id.raw) == 0 {
		return []byte("null"), nil
	}
	return id.raw, nil
}

// UnmarshalJSON accepts a JSON string or number (JSON-RPC 2.0's only permitted
// id types) and rejects everything else — null, object, array, bool — rather
// than silently coercing, so a malformed id surfaces as a parse error instead
// of a mismatched-type failure that discards the whole frame.
func (id *RPCID) UnmarshalJSON(b []byte) error {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 {
		return fmt.Errorf("rpc id: empty")
	}
	switch trimmed[0] {
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return fmt.Errorf("rpc id: invalid string id: %w", err)
		}
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		var n json.Number
		if err := json.Unmarshal(trimmed, &n); err != nil {
			return fmt.Errorf("rpc id: invalid numeric id: %w", err)
		}
	default:
		return fmt.Errorf("rpc id: must be a string or number, got %q", string(trimmed))
	}
	id.raw = append(json.RawMessage(nil), trimmed...)
	return nil
}

// Key returns a stable comparison key for the id: its canonical JSON token. A
// numeric `1` and a string `"1"` therefore never collide (`1` vs `"1"`).
func (id RPCID) Key() string {
	return string(id.raw)
}

// int64ID wraps a host-generated numeric id (the host allocates its own
// outbound request ids monotonically; plugins may use strings).
func int64ID(n int64) RPCID {
	return RPCID{raw: json.RawMessage(strconv.FormatInt(n, 10))}
}

// RPCError is a JSON-RPC error object.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

// Codec reads and writes length-prefixed frames carrying JSON-RPC on kind=0x01.
type Codec struct {
	fw *FrameWriter
}

// NewCodec creates a codec that frame-writes JSON-RPC messages to w.
func NewCodec(w io.Writer) *Codec {
	return &Codec{fw: NewFrameWriter(w)}
}

// WriteMessage serializes one JSON-RPC frame.
func (c *Codec) WriteMessage(msg Message) error {
	if msg.JSONRPC == "" {
		msg.JSONRPC = jsonRPCVersion
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal rpc message: %w", err)
	}
	if len(data) > domainplugin.MaxFrameBytes {
		return ErrFrameTooLarge
	}

	if err := c.fw.Write(domainplugin.FrameKindJSONRPC, 0, data); err != nil {
		return fmt.Errorf("write rpc frame: %w", err)
	}
	return nil
}

// ReadMessage reads one JSON-RPC (kind=0x01) frame from r.
func ReadMessage(r io.Reader) (Message, error) {
	hdr, payload, err := ReadFrame(r)
	if err != nil {
		return Message{}, err
	}
	if hdr.Kind != domainplugin.FrameKindJSONRPC {
		return Message{}, newProtocolViolation("unexpected frame kind 0x%02x on control-plane read", hdr.Kind)
	}
	return decodeRPCMessage(payload)
}

// decodeRPCMessage unmarshals the payload of a kind=0x01 frame. Split out of ReadMessage
// so Conn.readLoop can decode a kind=0x01 frame it already read off the shared stream
// without re-reading a frame header.
func decodeRPCMessage(payload []byte) (Message, error) {
	var msg Message
	if err := json.Unmarshal(payload, &msg); err != nil {
		return Message{}, fmt.Errorf("%w: %w", ErrParseError, err)
	}
	if msg.JSONRPC != "" && msg.JSONRPC != jsonRPCVersion {
		return Message{}, fmt.Errorf("unsupported jsonrpc version %q", msg.JSONRPC)
	}
	return msg, nil
}

// NewRequest builds a JSON-RPC request message. The host allocates numeric ids
// for its own outbound requests; a plugin's inbound ids are echoed via NewResponse.
func NewRequest(id int64, method string, params json.RawMessage) Message {
	rid := int64ID(id)
	msg := Message{JSONRPC: jsonRPCVersion, ID: &rid, Method: method}
	if len(params) > 0 {
		msg.Params = params
	}
	return msg
}

// NewNotification builds a JSON-RPC notification (no id).
func NewNotification(method string, params json.RawMessage) Message {
	msg := Message{JSONRPC: jsonRPCVersion, Method: method}
	if len(params) > 0 {
		msg.Params = params
	}
	return msg
}

// NewResponse builds a JSON-RPC success response, echoing the request id verbatim.
func NewResponse(id RPCID, result json.RawMessage) Message {
	return Message{JSONRPC: jsonRPCVersion, ID: &id, Result: result}
}

// NewErrorResponse builds a JSON-RPC error response, echoing the request id verbatim.
func NewErrorResponse(id RPCID, rpcErr RPCError) Message {
	return Message{JSONRPC: jsonRPCVersion, ID: &id, Error: &rpcErr}
}
