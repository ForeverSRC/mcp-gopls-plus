package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/ForeverSRC/mcp-gopls-plus/pkg/lsp/protocol"
)

func (c *GoplsClient) readerLoop() {
	defer close(c.readerDone)

	for {
		msg, err := c.transport.ReceiveMessage(c.readerCtx)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				c.logger.Warn("transport receive error", "error", err)
			}
			c.failAllPending(err)
			return
		}

		if msg == nil {
			continue
		}

		if msg.ID == nil {
			c.handleNotification(msg)
			continue
		}

		respID, ok := parseMessageID(msg.ID)
		if !ok {
			c.logger.Warn("response id has unexpected type", "id", msg.ID)
			continue
		}

		c.deliverResponse(respID, rpcResponse{msg: msg})
	}
}

func (c *GoplsClient) addPending(id int64, ch chan rpcResponse) error {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()

	if c.closed.Load() {
		return errors.New("client closed")
	}
	c.pending[id] = ch
	return nil
}

func (c *GoplsClient) removePending(id int64) {
	c.pendingMu.Lock()
	delete(c.pending, id)
	c.pendingMu.Unlock()
}

func (c *GoplsClient) deliverResponse(id int64, resp rpcResponse) {
	c.pendingMu.Lock()
	ch, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	c.pendingMu.Unlock()

	if !ok {
		return
	}

	select {
	case ch <- resp:
	default:
	}
}

func (c *GoplsClient) failAllPending(err error) {
	c.pendingMu.Lock()
	pending := c.pending
	c.pending = make(map[int64]chan rpcResponse)
	c.pendingMu.Unlock()

	if err == nil {
		err = errors.New("transport closed")
	}

	c.closed.Store(true)

	for _, ch := range pending {
		select {
		case ch <- rpcResponse{err: err}:
		default:
		}
	}
}

func (c *GoplsClient) call(ctx context.Context, method string, params any) (*protocol.JSONRPCMessage, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if c.callTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.callTimeout)
		defer cancel()
	}

	id := c.nextID.Add(1)
	respCh := make(chan rpcResponse, 1)

	if err := c.addPending(id, respCh); err != nil {
		return nil, err
	}

	if err := c.sendRequest(id, method, params); err != nil {
		c.removePending(id)
		return nil, err
	}

	select {
	case <-ctx.Done():
		c.removePending(id)
		return nil, ctx.Err()
	case resp := <-respCh:
		if resp.err != nil {
			return nil, resp.err
		}
		if resp.msg.Error != nil {
			return nil, fmt.Errorf("lsp error: %s (code %d)", resp.msg.Error.Message, resp.msg.Error.Code)
		}
		return resp.msg, nil
	}
}

func (c *GoplsClient) sendRequest(id int64, method string, params any) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	if c.closed.Load() {
		return errors.New("client closed")
	}
	if method != "initialize" && method != "shutdown" && !c.initialized.Load() {
		return errors.New("client not initialized")
	}

	req, err := protocol.NewRequest(id, method, params)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	if err := c.transport.SendMessage(req); err != nil {
		c.closed.Store(true)
		return fmt.Errorf("send request: %w", err)
	}

	c.logger.Debug("sent request", "method", method, "id", id)
	return nil
}

func (c *GoplsClient) notify(method string, params any) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	if c.closed.Load() {
		return errors.New("client closed")
	}

	notif, err := protocol.NewNotification(method, params)
	if err != nil {
		return fmt.Errorf("create notification: %w", err)
	}

	if err := c.transport.SendMessage(notif); err != nil {
		c.closed.Store(true)
		return fmt.Errorf("send notification: %w", err)
	}

	c.logger.Debug("sent notification", "method", method)
	return nil
}

func (c *GoplsClient) invoke(ctx context.Context, method string, params any) (*protocol.JSONRPCMessage, error) {
	if c.callOverride != nil {
		return c.callOverride(ctx, method, params)
	}
	return c.call(ctx, method, params)
}

func parseMessageID(id any) (int64, bool) {
	switch v := id.(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case json.Number:
		value, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return value, true
	default:
		return 0, false
	}
}

func pipeLogs(logger *slog.Logger, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		logger.Debug(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		logger.Warn("stderr stream error", "error", err)
	}
}
