package client

import (
	"context"
	"errors"
	"os"
)

// DidOpen sends a textDocument/didOpen notification (idempotent).
func (c *GoplsClient) DidOpen(ctx context.Context, uri, languageID, text string) error {
	_, err := c.ensureDocumentOpen(uri, languageID, text)
	return err
}

func (c *GoplsClient) ensureDocumentOpen(uri, languageID, text string) (bool, error) {
	if uri == "" {
		return false, errors.New("uri is required")
	}

	if _, alreadyOpen := c.openedDocs.LoadOrStore(uri, struct{}{}); alreadyOpen {
		return false, nil
	}

	if text == "" {
		if filePath, err := uriToPath(uri); err == nil {
			if data, readErr := os.ReadFile(filePath); readErr == nil {
				text = string(data)
			} else {
				c.logger.Warn("unable to read file contents", "uri", uri, "error", readErr)
			}
		}
	}

	params := map[string]any{
		"textDocument": map[string]any{
			"uri":        uri,
			"languageId": languageID,
			"version":    1,
			"text":       text,
		},
	}

	if err := c.notify("textDocument/didOpen", params); err != nil {
		c.openedDocs.Delete(uri)
		return false, err
	}

	return true, nil
}

// DidClose sends a textDocument/didClose notification and clears caches.
func (c *GoplsClient) DidClose(ctx context.Context, uri string) error {
	c.openedDocs.Delete(uri)
	c.diagnosticsMu.Lock()
	delete(c.diagnosticsCache, uri)
	c.diagnosticsMu.Unlock()
	params := map[string]any{
		"textDocument": map[string]any{
			"uri": uri,
		},
	}
	return c.notify("textDocument/didClose", params)
}
