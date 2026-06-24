package search

import (
	"context"

	"github.com/ForeverSRC/mcp-gopls-plus/pkg/lsp/protocol"
)

// Searcher exposes fuzzy search over workspace code metadata.
type Searcher interface {
	Search(context.Context, string, Options) ([]Result, error)
}

// ChangeAwareSearcher can refresh its index after file changes.
type ChangeAwareSearcher interface {
	Searcher
	ApplyFileChanges(context.Context, []protocol.FileEvent) error
}

// Options controls code search behavior.
type Options struct {
	TopK         int
	IncludeTests bool
}

// Result is a structured search hit returned to MCP clients.
type Result struct {
	File      string  `json:"file"`
	Symbol    string  `json:"symbol"`
	Kind      string  `json:"kind"`
	StartLine int     `json:"start_line"`
	EndLine   int     `json:"end_line"`
	Score     float64 `json:"score"`
	Summary   string  `json:"summary"`
}

// Backend is a concrete search backend implementation.
type Backend interface {
	Searcher
	Name() string
}

// ChunkSource produces searchable chunks from a workspace.
type ChunkSource interface {
	Collect(context.Context, string) ([]Chunk, error)
}

// QueryAnalyzer turns a raw query string into backend-ready tokens.
type QueryAnalyzer interface {
	Analyze(string) []string
}

// WorkspaceChunkSource is the default AST-aware chunk collector.
type WorkspaceChunkSource struct{}

// Collect implements ChunkSource with the built-in Go-aware chunker.
func (WorkspaceChunkSource) Collect(ctx context.Context, workspaceDir string) ([]Chunk, error) {
	return CollectWorkspaceChunks(ctx, workspaceDir)
}
