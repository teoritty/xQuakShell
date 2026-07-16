package ipc

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/pkg/safego"
)

const inboundRequestTimeout = 30 * time.Second

// RequestHandler handles incoming JSON-RPC requests from the plugin process.
type RequestHandler func(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, *RPCError)

// Conn multiplexes JSON-RPC over a single duplex NDJSON stream.
type Conn struct {
	enc         *Codec
	reader      *bufio.Reader
	writeCloser io.Closer
	readCloser  io.Closer
	pending     map[int64]chan messageResult
	nextID      atomic.Int64
	onNotify    func(method string, params json.RawMessage)
	onRequest   RequestHandler
	mux         *channelMux
	closeCh     chan struct{}
	closeOnce   sync.Once
	wg          sync.WaitGroup
	readErr     error
	mu          sync.Mutex
}

type messageResult struct {
	msg Message
	err error
}

// NewConn creates a connection. readFrom is plugin stdout; writeTo is plugin stdin.
func NewConn(readFrom io.Reader, writeTo io.Writer, onNotify func(string, json.RawMessage), onRequest RequestHandler) *Conn {
	var wc io.Closer
	if c, ok := writeTo.(io.Closer); ok {
		wc = c
	}
	// readFrom is the plugin's stdout pipe in production (an io.ReadCloser). Close needs it to
	// unpark the read loop; see Conn.Close.
	var rc io.Closer
	if c, ok := readFrom.(io.Closer); ok {
		rc = c
	}
	c := &Conn{
		enc:         NewCodec(writeTo),
		reader:      bufio.NewReader(readFrom),
		writeCloser: wc,
		readCloser:  rc,
		pending:     make(map[int64]chan messageResult),
		onNotify:    onNotify,
		onRequest:   onRequest,
		mux:         newChannelMux(),
		closeCh:     make(chan struct{}),
	}
	c.wg.Add(1)
	safego.GoNamed("ipc.readLoop", c.readLoop)
	return c
}

// Call sends a JSON-RPC request and waits for the matching response.
func (c *Conn) Call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	ch := make(chan messageResult, 1)

	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	if err := c.enc.WriteMessage(NewRequest(id, method, params)); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if res.err != nil {
			return nil, res.err
		}
		if res.msg.Error != nil {
			return nil, res.msg.Error
		}
		return res.msg.Result, nil
	}
}

// Notify sends a JSON-RPC notification (no response expected).
func (c *Conn) Notify(method string, params json.RawMessage) error {
	return c.enc.WriteMessage(NewNotification(method, params))
}

// WriteBinary emits a kind=0x02 binary channel data frame on the same serialized writer
// as JSON-RPC, so the two never interleave mid-frame on the wire.
func (c *Conn) WriteBinary(channelID uint32, payload []byte) error {
	return c.enc.fw.Write(domainplugin.FrameKindBinary, channelID, payload)
}

// WriteCredit emits a kind=0x03 credit/window-update frame. Payload layout is fixed per
// ADR-011: 4B channelId + 4B credit, 8 bytes total, no subtype byte.
func (c *Conn) WriteCredit(channelID uint32, credit uint32) error {
	payload := make([]byte, 8)
	binary.BigEndian.PutUint32(payload[0:4], channelID)
	binary.BigEndian.PutUint32(payload[4:8], credit)
	return c.enc.fw.Write(domainplugin.FrameKindCredit, channelID, payload)
}

// CloseWrite closes the plugin stdin side so the child can exit on EOF.
func (c *Conn) CloseWrite() {
	if c.writeCloser != nil {
		_ = c.writeCloser.Close()
	}
}

// Close stops the read loop and waits for it to exit.
//
// Closing closeCh alone is not enough to end the read loop: it parks in ReadFrame until the
// plugin sends bytes or the pipe yields, and closeCh is only consulted between frames. A
// plugin that is hung, wedged, or hostile simply never sends the frame that would let the
// loop notice, so Close must also close the read side to unpark it. Without that, Close
// blocks forever on wg.Wait — and since managedProcess.closeResources calls Close before it
// kills the child, the kill that would have freed the pipe is unreachable.
//
// The read side is the child's stdout pipe; closing it early is safe alongside the reaper's
// (*exec.Cmd).Wait, which closes its own descriptors and tolerates one already closed.
func (c *Conn) Close() {
	c.closeOnce.Do(func() {
		close(c.closeCh)
		if c.readCloser != nil {
			_ = c.readCloser.Close()
		}
	})
	c.wg.Wait()
}

func (c *Conn) readLoop() {
	defer c.wg.Done()

	for {
		hdr, payload, err := ReadFrame(c.reader)
		if err != nil {
			c.failReadLoop(err, false)
			return
		}

		if hdr.Kind != domainplugin.FrameKindJSONRPC {
			select {
			case <-c.closeCh:
				return
			default:
			}
			if dispatchErr := c.mux.Dispatch(hdr, payload); dispatchErr != nil {
				c.failReadLoop(dispatchErr, false)
				return
			}
			continue
		}

		msg, err := decodeRPCMessage(payload)
		if err != nil {
			c.failReadLoop(err, IsParseError(err))
			return
		}

		select {
		case <-c.closeCh:
			return
		default:
		}

		if msg.ID != nil {
			if msg.Method == "" && msg.Error == nil && msg.Result == nil {
				_ = c.enc.WriteMessage(NewErrorResponse(*msg.ID, RPCError{Code: -32600, Message: "Invalid Request"}))
				continue
			}
			c.mu.Lock()
			ch, ok := c.pending[*msg.ID]
			c.mu.Unlock()
			if ok {
				ch <- messageResult{msg: msg}
				continue
			}
			if msg.Method != "" && c.onRequest != nil {
				c.wg.Add(1)
				reqID := *msg.ID
				reqMethod := msg.Method
				reqParams := append(json.RawMessage(nil), msg.Params...)
				safego.GoNamed("ipc.handleRequest", func() {
					defer c.wg.Done()
					c.handleIncomingRequest(reqID, reqMethod, reqParams)
				})
				continue
			}
		}

		if msg.Method != "" && c.onNotify != nil {
			c.onNotify(msg.Method, msg.Params)
		}
	}
}

// failReadLoop handles a terminal read-loop error (frame I/O failure, JSON-RPC parse
// error, or a channel-mux protocol violation) uniformly: any of them ends the connection,
// per ADR-011 §2a fail-fast.
func (c *Conn) failReadLoop(err error, isParseError bool) {
	if isParseError {
		_ = c.enc.WriteMessage(NewParseErrorResponse())
	}
	select {
	case <-c.closeCh:
		return
	default:
	}
	c.mu.Lock()
	c.readErr = err
	for id, ch := range c.pending {
		ch <- messageResult{err: err}
		delete(c.pending, id)
	}
	c.mu.Unlock()
}

func (c *Conn) handleIncomingRequest(id int64, method string, params json.RawMessage) {
	ctx, cancel := context.WithTimeout(context.Background(), inboundRequestTimeout)
	defer cancel()
	result, rpcErr := c.onRequest(ctx, method, params)
	var msg Message
	if rpcErr != nil {
		msg = NewErrorResponse(id, *rpcErr)
	} else {
		msg = NewResponse(id, result)
	}
	if err := c.enc.WriteMessage(msg); err != nil {
		c.mu.Lock()
		c.readErr = err
		c.mu.Unlock()
	}
}

// ReadError returns the terminal read error, if any.
func (c *Conn) ReadError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.readErr
}

// EncodeResult marshals a Go value into json.RawMessage.
func EncodeResult(v any) (json.RawMessage, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encode rpc result: %w", err)
	}
	return data, nil
}

// EncodeParams marshals call params.
func EncodeParams(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encode rpc params: %w", err)
	}
	return data, nil
}
