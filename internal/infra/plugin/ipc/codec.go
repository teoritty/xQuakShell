package ipc

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	domainplugin "ssh-client/internal/domain/plugin"
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
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
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

// Codec reads and writes NDJSON frames.
type Codec struct {
	w  io.Writer
	mu sync.Mutex
}

// NewCodec creates a codec that writes newline-delimited JSON to w.
func NewCodec(w io.Writer) *Codec {
	return &Codec{w: w}
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

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := c.w.Write(data); err != nil {
		return fmt.Errorf("write rpc frame: %w", err)
	}
	if _, err := c.w.Write([]byte{'\n'}); err != nil {
		return fmt.Errorf("write rpc newline: %w", err)
	}
	return nil
}

// ReadMessage reads one JSON-RPC frame from r.
func ReadMessage(r *bufio.Reader) (Message, error) {
	var line []byte
	for {
		fragment, err := r.ReadSlice('\n')
		line = append(line, fragment...)
		if len(line) > domainplugin.MaxFrameBytes {
			return Message{}, ErrFrameTooLarge
		}
		if err == nil {
			break
		}
		if err != bufio.ErrBufferFull {
			return Message{}, fmt.Errorf("read rpc frame: %w", err)
		}
	}
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return Message{}, fmt.Errorf("empty rpc frame")
	}

	var msg Message
	if err := json.Unmarshal(line, &msg); err != nil {
		return Message{}, fmt.Errorf("%w: %w", ErrParseError, err)
	}
	if msg.JSONRPC != "" && msg.JSONRPC != jsonRPCVersion {
		return Message{}, fmt.Errorf("unsupported jsonrpc version %q", msg.JSONRPC)
	}
	return msg, nil
}

// NewRequest builds a JSON-RPC request message.
func NewRequest(id int64, method string, params json.RawMessage) Message {
	msg := Message{JSONRPC: jsonRPCVersion, ID: &id, Method: method}
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

// NewResponse builds a JSON-RPC success response.
func NewResponse(id int64, result json.RawMessage) Message {
	return Message{JSONRPC: jsonRPCVersion, ID: &id, Result: result}
}

// NewErrorResponse builds a JSON-RPC error response.
func NewErrorResponse(id int64, rpcErr RPCError) Message {
	return Message{JSONRPC: jsonRPCVersion, ID: &id, Error: &rpcErr}
}
