// Package pluginhost provides a minimal JSON-RPC loop for integration test plugins.
package pluginhost

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const (
	jsonRPCVersion   = "2.0"
	defaultCallTimeout = 5 * time.Second
	maxFrameBytes    = 256 << 10
)

// Message is a JSON-RPC 2.0 frame.
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

// Handler handles an incoming JSON-RPC request from the core host.
type Handler func(params json.RawMessage) (any, error)

// NotificationHandler handles host→plugin notifications.
type NotificationHandler func(params json.RawMessage)

// Host runs the plugin-side JSON-RPC loop on stdin/stdout.
type Host struct {
	in        *bufio.Reader
	out       *jsonWriter
	handlers  map[string]Handler
	notify    map[string]NotificationHandler
	pending   map[int64]chan Message
	nextID    atomic.Int64
	pendingMu sync.Mutex
	closeCh   chan struct{}
	wg        sync.WaitGroup
}

// NewHost creates a plugin host using os.Stdin and os.Stdout.
func NewHost() *Host {
	return NewHostFromStreams(os.Stdin, os.Stdout)
}

// NewHostFromStreams creates a plugin host for testing.
func NewHostFromStreams(in io.Reader, out io.Writer) *Host {
	h := &Host{
		in:       bufio.NewReader(in),
		out:      newJSONWriter(out),
		handlers: make(map[string]Handler),
		notify:   make(map[string]NotificationHandler),
		pending:  make(map[int64]chan Message),
		closeCh:  make(chan struct{}),
	}
	h.wg.Add(1)
	go h.readLoop()
	return h
}

// Register binds a request method handler.
func (h *Host) Register(method string, handler Handler) {
	h.handlers[method] = handler
}

// RegisterNotification binds a host notification handler.
func (h *Host) RegisterNotification(method string, handler NotificationHandler) {
	h.notify[method] = handler
}

// CallCore sends a JSON-RPC request to the core host.
func (h *Host) CallCore(method string, params any) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultCallTimeout)
	defer cancel()
	return h.CallCoreContext(ctx, method, params)
}

// CallCoreContext sends a JSON-RPC request to the core host and waits until ctx expires.
func (h *Host) CallCoreContext(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	id := h.nextID.Add(1)
	ch := make(chan Message, 1)

	h.pendingMu.Lock()
	h.pending[id] = ch
	h.pendingMu.Unlock()
	defer func() {
		h.pendingMu.Lock()
		delete(h.pending, id)
		h.pendingMu.Unlock()
	}()

	var rawParams json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		rawParams = data
	}

	if err := h.out.WriteMessage(Message{
		JSONRPC: jsonRPCVersion,
		ID:      &id,
		Method:  method,
		Params:  rawParams,
	}); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("host connection closed")
		}
		if msg.Error != nil {
			return nil, fmt.Errorf("rpc error %d: %s", msg.Error.Code, msg.Error.Message)
		}
		return msg.Result, nil
	}
}

// Run blocks until stdin closes.
func (h *Host) Run() error {
	<-h.closeCh
	h.wg.Wait()
	return nil
}

func (h *Host) readLoop() {
	defer h.wg.Done()
	defer close(h.closeCh)

	for {
		msg, err := readMessage(h.in)
		if err != nil {
			if isParseError(err) {
				_ = h.out.WriteMessage(Message{
					JSONRPC: jsonRPCVersion,
					Error:   &RPCError{Code: -32700, Message: "Parse error"},
				})
			}
			return
		}

		if msg.ID != nil {
			h.pendingMu.Lock()
			ch, ok := h.pending[*msg.ID]
			h.pendingMu.Unlock()
			if ok {
				ch <- msg
				continue
			}
		}

		if msg.Method == "" {
			continue
		}

		if msg.ID == nil {
			if handler, ok := h.notify[msg.Method]; ok {
				handler(msg.Params)
			}
			continue
		}

		handler, ok := h.handlers[msg.Method]
		if !ok {
			_ = h.writeError(*msg.ID, -32601, "method not found")
			continue
		}

		result, err := handler(msg.Params)
		if err != nil {
			_ = h.writeError(*msg.ID, -32603, "request failed")
			continue
		}

		data, err := json.Marshal(result)
		if err != nil {
			_ = h.writeError(*msg.ID, -32603, "marshal result failed")
			continue
		}
		_ = h.out.WriteMessage(Message{
			JSONRPC: jsonRPCVersion,
			ID:      msg.ID,
			Result:  data,
		})
	}
}

func (h *Host) writeError(id int64, code int, message string) error {
	return h.out.WriteMessage(Message{
		JSONRPC: jsonRPCVersion,
		ID:      &id,
		Error:   &RPCError{Code: code, Message: message},
	})
}

type jsonWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func newJSONWriter(w io.Writer) *jsonWriter {
	return &jsonWriter{w: w}
}

func (jw *jsonWriter) WriteMessage(msg Message) error {
	jw.mu.Lock()
	defer jw.mu.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if len(data) > maxFrameBytes {
		return fmt.Errorf("rpc frame exceeds %d bytes", maxFrameBytes)
	}
	if _, err := jw.w.Write(data); err != nil {
		return err
	}
	_, err = jw.w.Write([]byte{'\n'})
	return err
}

func readMessage(r *bufio.Reader) (Message, error) {
	var line []byte
	for {
		fragment, err := r.ReadSlice('\n')
		line = append(line, fragment...)
		if len(line) > maxFrameBytes {
			return Message{}, fmt.Errorf("rpc frame exceeds %d bytes", maxFrameBytes)
		}
		if err == nil {
			break
		}
		if err != bufio.ErrBufferFull {
			return Message{}, err
		}
	}
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return Message{}, fmt.Errorf("empty rpc frame")
	}
	var msg Message
	if err := json.Unmarshal(line, &msg); err != nil {
		return Message{}, fmt.Errorf("%w: %w", errParseError, err)
	}
	return msg, nil
}

var errParseError = errors.New("jsonrpc parse error")

func isParseError(err error) bool {
	return errors.Is(err, errParseError)
}
