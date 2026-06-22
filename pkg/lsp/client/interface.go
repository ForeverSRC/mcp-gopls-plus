package client

import (
	"context"

	"github.com/ForeverSRC/mcp-gopls-plus/pkg/lsp/protocol"
)

// DiagnosticsHandler is invoked whenever gopls publishes diagnostics.
type DiagnosticsHandler func(protocol.PublishDiagnosticsParams)

// LSPClient defines the interface for an LSP client.
type LSPClient interface {
	// Protocol basics
	Initialize(ctx context.Context) error
	Shutdown(ctx context.Context) error
	Close(ctx context.Context) error

	// Code navigation
	GoToDefinition(ctx context.Context, uri string, line, character int) ([]protocol.Location, error)
	FindReferences(ctx context.Context, uri string, line, character int, includeDeclaration bool) ([]protocol.Location, error)

	// Diagnostics
	GetDiagnostics(ctx context.Context, uri string) ([]protocol.Diagnostic, error)

	// Document management
	DidOpen(ctx context.Context, uri, languageID, text string) error
	DidClose(ctx context.Context, uri string) error

	// Advanced features
	GetHover(ctx context.Context, uri string, line, character int) (string, error)
	GetCompletion(ctx context.Context, uri string, line, character int) ([]string, error)

	DocumentFormatting(ctx context.Context, uri string) ([]protocol.TextEdit, error)
	Rename(ctx context.Context, uri string, line, character int, newName string) (*protocol.WorkspaceEdit, error)
	CodeActions(ctx context.Context, uri string, rng protocol.Range) ([]protocol.CodeAction, error)
	WorkspaceSymbols(ctx context.Context, query string) ([]protocol.SymbolInformation, error)
	DocumentSymbols(ctx context.Context, uri string) ([]protocol.DocumentSymbol, error)

	// Watched files
	NotifyDidChangeWatchedFiles(ctx context.Context, changes []protocol.FileEvent) error

	// Observability
	OnDiagnostics(handler DiagnosticsHandler) func()
}
