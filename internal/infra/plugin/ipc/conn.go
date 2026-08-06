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

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/pkg/safego"
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
	// pending maps an in-flight request id to its size-1 reply channel. Ownership
	// invariant: whoever removes an entry from pending under c.mu becomes that
	// channel's sole sender, and does the send outside the lock. Call never removes
	// an entry to claim ownership — its deferred delete only cleans up the no-answer
	// path (ctx expired before dispatch or failReadLoop ever claimed the entry) and
	// must stay a no-op everywhere else. Breaking this invariant reintroduces a real
	// deadlock: two senders racing a size-1 buffered channel, one of them inside
	// failReadLoop while holding c.mu.
	pending   map[string]chan messageResult
	nextID    atomic.Int64
	onNotify  func(method string, params json.RawMessage)
	onRequest RequestHandler
	mux       *channelMux
	// channelThroughputKbps is the manifest's channel.maxThroughputKbps for this plugin,
	// applied to every channel this conn opens.
	channelThroughputKbps int
	closeCh               chan struct{}
	closeOnce             sync.Once
	wg                    sync.WaitGroup
	readErr               error
	mu                    sync.Mutex
}

type messageResult struct {
	msg Message
	err error
}

// NewConn creates a connection. readFrom is plugin stdout; writeTo is plugin stdin.
// channelThroughputKbps is the manifest's declared channel bandwidth cap, applied to every
// channel opened on this conn; 0 selects the host default.
func NewConn(readFrom io.Reader, writeTo io.Writer, onNotify func(string, json.RawMessage), onRequest RequestHandler, channelThroughputKbps int) *Conn {
	if channelThroughputKbps <= 0 {
		channelThroughputKbps = domainplugin.DefaultChannelThroughputKbps
	}
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
		enc:                   NewCodec(writeTo),
		reader:                bufio.NewReader(readFrom),
		writeCloser:           wc,
		readCloser:            rc,
		pending:               make(map[string]chan messageResult),
		onNotify:              onNotify,
		onRequest:             onRequest,
		closeCh:               make(chan struct{}),
		channelThroughputKbps: channelThroughputKbps,
	}
	// The mux's channels emit through this conn's serialized writer, so channel data and
	// JSON-RPC never interleave mid-frame.
	c.mux = newChannelMux(c.WriteBinary)
	c.wg.Add(1)
	safego.GoNamed("ipc.readLoop", c.readLoop)
	return c
}

// Call sends a JSON-RPC request and waits for the matching response.
func (c *Conn) Call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	key := int64ID(id).Key()
	ch := make(chan messageResult, 1)

	c.mu.Lock()
	c.pending[key] = ch
	c.mu.Unlock()

	// Cleanup for the no-answer path only: if a dispatch or failReadLoop already
	// claimed this channel, the entry is gone and this delete is a no-op. Never
	// remove it — it is what keeps an abandoned pending entry from leaking when
	// ctx expires before any response arrives.
	defer func() {
		c.mu.Lock()
		delete(c.pending, key)
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
		// No plugin is left to spare, so every channel goes at once rather than waiting out
		// its grace period. This also unparks any backend pump still sitting in Recv, which
		// would otherwise keep the process's goroutines alive past its connection.
		c.mux.ReleaseAll()
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

		// What a message IS decides where it goes; its id only says which one. Both peers number
		// their own requests from 1, so the two id spaces overlap by construction — matching an
		// inbound request against c.pending would hand the plugin's request #3 to whoever is
		// waiting on the host's call #3, leaving the request unanswered and the call resolved
		// with a request for a result. `method` is the only thing that tells them apart
		// (JSON-RPC 2.0 §4, §5).
		if msg.Method != "" {
			if msg.ID == nil {
				if c.onNotify != nil {
					c.onNotify(msg.Method, msg.Params)
				}
				continue
			}
			if c.onRequest != nil {
				c.wg.Add(1)
				reqID := *msg.ID
				reqMethod := msg.Method
				reqParams := append(json.RawMessage(nil), msg.Params...)
				safego.GoNamed("ipc.handleRequest", func() {
					defer c.wg.Done()
					c.handleIncomingRequest(reqID, reqMethod, reqParams)
				})
			}
			continue
		}

		if msg.ID != nil {
			if msg.Error == nil && msg.Result == nil {
				_ = c.enc.WriteMessage(NewErrorResponse(*msg.ID, RPCError{Code: -32600, Message: "Invalid Request"}))
				continue
			}
			key := msg.ID.Key()
			c.mu.Lock()
			ch, ok := c.pending[key]
			delete(c.pending, key) // ownership transfers here, under the same lock
			c.mu.Unlock()
			if ok {
				ch <- messageResult{msg: msg} // buffer is guaranteed empty: sole sender
			}
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
	pending := c.pending
	c.pending = make(map[string]chan messageResult)
	c.mu.Unlock()

	// Outside the lock. Ownership of every entry transferred to us the instant we swapped
	// the map out above, so each channel has exactly one sender here — the send can't block
	// forever even if Call already gave up on it via ctx.Done() and nobody is left to read.
	for _, ch := range pending {
		ch <- messageResult{err: err}
	}
}

func (c *Conn) handleIncomingRequest(id RPCID, method string, params json.RawMessage) {
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
		return
	}
	// The channel.open reply is now on the wire ahead of any data frame the backend's pump has
	// buffered (the pump blocks in channel.Send until here). Only now may host->plugin data
	// flow: before the reply, the plugin has not registered the channelId and would kill itself
	// over a frame for a channel it does not know is open.
	if rpcErr == nil && method == "channel.open" {
		if chID := channelIDFromOpenResult(result); chID != 0 {
			c.mux.MarkOpened(chID)
		}
	}
}

// channelIDFromOpenResult extracts the channelId a successful channel.open reply carries, or 0
// if the result is not the expected {"channelId":<uint32>} shape.
func channelIDFromOpenResult(result json.RawMessage) uint32 {
	var r struct {
		ChannelID uint32 `json:"channelId"`
	}
	if err := json.Unmarshal(result, &r); err != nil {
		return 0
	}
	return r.ChannelID
}

// ReadError reports why the read loop stopped, under the same mutex the loop
// stores it with. It stays readable after the connection is closed, which is
// what lets a caller tell an orderly shutdown from a protocol failure.
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
