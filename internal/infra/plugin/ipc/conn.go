package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

const inboundRequestTimeout = 30 * time.Second

// RequestHandler handles incoming JSON-RPC requests from the plugin process.
type RequestHandler func(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, *RPCError)

// Conn multiplexes JSON-RPC over a single duplex NDJSON stream.
type Conn struct {
	enc         *Codec
	reader      *bufio.Reader
	writeCloser io.Closer
	pending     map[int64]chan messageResult
	nextID     atomic.Int64
	onNotify   func(method string, params json.RawMessage)
	onRequest  RequestHandler
	closeCh    chan struct{}
	wg         sync.WaitGroup
	readErr    error
	mu         sync.Mutex
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
	c := &Conn{
		enc:         NewCodec(writeTo),
		reader:      bufio.NewReader(readFrom),
		writeCloser: wc,
		pending:     make(map[int64]chan messageResult),
		onNotify:  onNotify,
		onRequest: onRequest,
		closeCh:   make(chan struct{}),
	}
	c.wg.Add(1)
	go c.readLoop()
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

// CloseWrite closes the plugin stdin side so the child can exit on EOF.
func (c *Conn) CloseWrite() {
	if c.writeCloser != nil {
		_ = c.writeCloser.Close()
	}
}

// Close stops the read loop. Does not close underlying pipes.
func (c *Conn) Close() {
	select {
	case <-c.closeCh:
	default:
		close(c.closeCh)
	}
	c.wg.Wait()
}

func (c *Conn) readLoop() {
	defer c.wg.Done()

	for {
		msg, err := ReadMessage(c.reader)
		if err != nil {
			if IsParseError(err) {
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
				c.handleIncomingRequest(*msg.ID, msg.Method, msg.Params)
				continue
			}
		}

		if msg.Method != "" && c.onNotify != nil {
			c.onNotify(msg.Method, msg.Params)
		}
	}
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
