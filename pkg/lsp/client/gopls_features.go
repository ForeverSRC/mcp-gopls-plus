package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ForeverSRC/mcp-gopls-plus/pkg/lsp/protocol"
)

// GoToDefinition implements LSPClient.
func (c *GoplsClient) GoToDefinition(ctx context.Context, uri string, line, character int) ([]protocol.Location, error) {
	params := protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Position: protocol.Position{
			Line:      line,
			Character: character,
		},
	}

	resp, err := c.invoke(ctx, "textDocument/definition", params)
	if err != nil {
		return nil, err
	}

	var locations []protocol.Location
	if err := resp.ParseResult(&locations); err != nil {
		return nil, fmt.Errorf("decode definition: %w", err)
	}
	return locations, nil
}

// FindReferences implements LSPClient.
func (c *GoplsClient) FindReferences(ctx context.Context, uri string, line, character int, includeDeclaration bool) ([]protocol.Location, error) {
	params := protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position: protocol.Position{
				Line:      line,
				Character: character,
			},
		},
		Context: protocol.ReferenceContext{
			IncludeDeclaration: includeDeclaration,
		},
	}

	resp, err := c.invoke(ctx, "textDocument/references", params)
	if err != nil {
		return nil, err
	}

	var locations []protocol.Location
	if err := resp.ParseResult(&locations); err != nil {
		return nil, fmt.Errorf("decode references: %w", err)
	}
	return locations, nil
}

// NotifyDidChangeWatchedFiles implements LSPClient.
func (c *GoplsClient) NotifyDidChangeWatchedFiles(ctx context.Context, changes []protocol.FileEvent) error {
	params := protocol.DidChangeWatchedFilesParams{Changes: changes}
	return c.notify("workspace/didChangeWatchedFiles", params)
}

// GetHover implements LSPClient.
func (c *GoplsClient) GetHover(ctx context.Context, uri string, line, character int) (string, error) {
	opened, err := c.ensureDocumentOpen(uri, "go", "")
	if err != nil {
		return "", err
	}
	if opened {
		defer func() {
			_ = c.DidClose(ctx, uri)
		}()
	}
	params := protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Position: protocol.Position{
			Line:      line,
			Character: character,
		},
	}

	resp, err := c.invoke(ctx, "textDocument/hover", params)
	if err != nil {
		return "", err
	}

	if resp.Result == nil || string(resp.Result) == "null" {
		return "", errors.New("no hover information available")
	}

	var payload map[string]any
	if parseErr := resp.ParseResult(&payload); parseErr != nil {
		return "", fmt.Errorf("decode hover: %w", parseErr)
	}

	if contents, ok := payload["contents"]; ok {
		switch v := contents.(type) {
		case string:
			return v, nil
		case map[string]any:
			if value, ok := v["value"].(string); ok {
				return value, nil
			}
		case []any:
			if len(v) > 0 {
				if first, ok := v[0].(map[string]any); ok {
					if value, ok := first["value"].(string); ok {
						return value, nil
					}
				}
			}
		}
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// GetCompletion implements LSPClient.
func (c *GoplsClient) GetCompletion(ctx context.Context, uri string, line, character int) ([]string, error) {
	opened, err := c.ensureDocumentOpen(uri, "go", "")
	if err != nil {
		return nil, err
	}
	if opened {
		defer func() {
			_ = c.DidClose(ctx, uri)
		}()
	}
	params := protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Position: protocol.Position{
			Line:      line,
			Character: character,
		},
	}

	resp, err := c.invoke(ctx, "textDocument/completion", params)
	if err != nil {
		return nil, err
	}

	var payload map[string]any
	if err := resp.ParseResult(&payload); err != nil {
		return nil, fmt.Errorf("decode completion: %w", err)
	}

	var completions []string
	if items, ok := payload["items"].([]any); ok {
		for _, item := range items {
			if dict, ok := item.(map[string]any); ok {
				if label, ok := dict["label"].(string); ok {
					completions = append(completions, label)
				}
			}
		}
	}

	return completions, nil
}

func (c *GoplsClient) DocumentFormatting(ctx context.Context, uri string) ([]protocol.TextEdit, error) {
	params := protocol.DocumentFormattingParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Options: protocol.FormattingOptions{
			TabSize:      4,
			InsertSpaces: true,
		},
	}

	resp, err := c.invoke(ctx, "textDocument/formatting", params)
	if err != nil {
		return nil, err
	}

	var edits []protocol.TextEdit
	if err := resp.ParseResult(&edits); err != nil {
		return nil, fmt.Errorf("decode formatting edits: %w", err)
	}
	return edits, nil
}

func (c *GoplsClient) Rename(ctx context.Context, uri string, line, character int, newName string) (*protocol.WorkspaceEdit, error) {
	params := protocol.RenameParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position: protocol.Position{
				Line:      line,
				Character: character,
			},
		},
		NewName: newName,
	}

	resp, err := c.invoke(ctx, "textDocument/rename", params)
	if err != nil {
		return nil, err
	}

	var edit protocol.WorkspaceEdit
	if err := resp.ParseResult(&edit); err != nil {
		return nil, fmt.Errorf("decode rename result: %w", err)
	}
	return &edit, nil
}

func (c *GoplsClient) CodeActions(ctx context.Context, uri string, rng protocol.Range) ([]protocol.CodeAction, error) {
	params := protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Range:        rng,
		Context: protocol.CodeActionContext{
			Diagnostics: []protocol.Diagnostic{},
		},
	}

	resp, err := c.invoke(ctx, "textDocument/codeAction", params)
	if err != nil {
		return nil, err
	}

	var actions []protocol.CodeAction
	if err := resp.ParseResult(&actions); err != nil {
		return nil, fmt.Errorf("decode code actions: %w", err)
	}
	return actions, nil
}

func (c *GoplsClient) WorkspaceSymbols(ctx context.Context, query string) ([]protocol.SymbolInformation, error) {
	params := protocol.WorkspaceSymbolParams{Query: query}
	resp, err := c.invoke(ctx, "workspace/symbol", params)
	if err != nil {
		return nil, err
	}

	var symbols []protocol.SymbolInformation
	if err := resp.ParseResult(&symbols); err != nil {
		return nil, fmt.Errorf("decode workspace symbols: %w", err)
	}
	return symbols, nil
}

func (c *GoplsClient) DocumentSymbols(ctx context.Context, uri string) ([]protocol.DocumentSymbol, error) {
	params := protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	}
	resp, err := c.invoke(ctx, "textDocument/documentSymbol", params)
	if err != nil {
		return nil, err
	}

	var symbols []protocol.DocumentSymbol
	if err := resp.ParseResult(&symbols); err != nil {
		return nil, fmt.Errorf("decode document symbols: %w", err)
	}
	return symbols, nil
}
