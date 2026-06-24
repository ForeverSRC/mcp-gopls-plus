package client

import (
	"context"
	"encoding/json"

	"github.com/ForeverSRC/mcp-gopls-plus/pkg/lsp/protocol"
)

func (c *GoplsClient) waitForDiagnostics(ctx context.Context, uri string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	c.diagnosticsMu.Lock()
	if _, ok := c.diagnosticsCache[uri]; ok {
		c.diagnosticsMu.Unlock()
		return nil
	}

	ch := make(chan struct{}, 1)
	c.diagnosticsWaiters[uri] = append(c.diagnosticsWaiters[uri], ch)
	c.diagnosticsMu.Unlock()

	select {
	case <-ctx.Done():
		c.removeDiagnosticsWaiter(uri, ch)
		return ctx.Err()
	case <-ch:
		return nil
	}
}

func (c *GoplsClient) removeDiagnosticsWaiter(uri string, target chan struct{}) {
	c.diagnosticsMu.Lock()
	defer c.diagnosticsMu.Unlock()

	waiters := c.diagnosticsWaiters[uri]
	for i, ch := range waiters {
		if ch == target {
			waiters = append(waiters[:i], waiters[i+1:]...)
			break
		}
	}

	if len(waiters) == 0 {
		delete(c.diagnosticsWaiters, uri)
		return
	}

	c.diagnosticsWaiters[uri] = waiters
}

// GetDiagnostics returns the cached diagnostics for the provided URI.
func (c *GoplsClient) GetDiagnostics(ctx context.Context, uri string) ([]protocol.Diagnostic, error) {
	opened, err := c.ensureDocumentOpen(uri, "go", "")
	if err != nil {
		return nil, err
	}
	if opened {
		defer func() {
			_ = c.DidClose(ctx, uri)
		}()
	}

	if err := c.waitForDiagnostics(ctx, uri); err != nil {
		return nil, err
	}

	c.diagnosticsMu.RLock()
	defer c.diagnosticsMu.RUnlock()

	items := c.diagnosticsCache[uri]
	cloned := make([]protocol.Diagnostic, len(items))
	copy(cloned, items)
	return cloned, nil
}

// OnDiagnostics registers a handler for publishDiagnostics notifications.
func (c *GoplsClient) OnDiagnostics(handler DiagnosticsHandler) func() {
	if handler == nil {
		return func() {}
	}

	id := c.handlerCounter.Add(1)
	c.diagnosticsMu.Lock()
	c.diagnosticsHandlers[id] = handler
	c.diagnosticsMu.Unlock()

	return func() {
		c.diagnosticsMu.Lock()
		delete(c.diagnosticsHandlers, id)
		c.diagnosticsMu.Unlock()
	}
}

func (c *GoplsClient) handleNotification(msg *protocol.JSONRPCMessage) {
	switch msg.Method {
	case "textDocument/publishDiagnostics":
		var params protocol.PublishDiagnosticsParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			c.logger.Warn("failed to decode diagnostics", "error", err)
			return
		}
		c.updateDiagnostics(params)
	default:
		c.logger.Debug("ignoring notification", "method", msg.Method)
	}
}

func (c *GoplsClient) updateDiagnostics(params protocol.PublishDiagnosticsParams) {
	c.diagnosticsMu.Lock()
	c.diagnosticsCache[params.URI] = params.Diagnostics

	handlers := make([]DiagnosticsHandler, 0, len(c.diagnosticsHandlers))
	for _, h := range c.diagnosticsHandlers {
		handlers = append(handlers, h)
	}
	waiters := c.diagnosticsWaiters[params.URI]
	if len(waiters) > 0 {
		delete(c.diagnosticsWaiters, params.URI)
	}
	c.diagnosticsMu.Unlock()

	for _, waiter := range waiters {
		select {
		case waiter <- struct{}{}:
		default:
		}
	}

	for _, handler := range handlers {
		handler(params)
	}
}
