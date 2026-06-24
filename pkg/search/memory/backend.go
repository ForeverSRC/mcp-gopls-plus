package memory

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"sync"

	"github.com/ForeverSRC/mcp-gopls-plus/pkg/lsp/protocol"
	"github.com/ForeverSRC/mcp-gopls-plus/pkg/search"
)

// Backend is the current prototype backend backed by in-memory BM25 data.
type Backend struct {
	workspaceDir  string
	chunkSource   search.ChunkSource
	queryAnalyzer search.QueryAnalyzer
	mu            sync.RWMutex
	index         *index
}

var _ search.Backend = (*Backend)(nil)
var _ search.ChangeAwareSearcher = (*Backend)(nil)

// Build scans the workspace and constructs the in-memory backend.
func Build(ctx context.Context, workspaceDir string) (*Backend, error) {
	return BuildWithSource(ctx, workspaceDir, search.WorkspaceChunkSource{})
}

// BuildWithSource scans the workspace with the provided chunk source.
func BuildWithSource(ctx context.Context, workspaceDir string, source search.ChunkSource) (*Backend, error) {
	return BuildWithSourceAndAnalyzer(ctx, workspaceDir, source, newQueryAnalyzer())
}

// BuildWithSourceAndAnalyzer scans the workspace with explicit source and query analyzer components.
func BuildWithSourceAndAnalyzer(ctx context.Context, workspaceDir string, source search.ChunkSource, analyzer search.QueryAnalyzer) (*Backend, error) {
	if workspaceDir == "" {
		return nil, fmt.Errorf("workspace dir is required")
	}
	if source == nil {
		return nil, fmt.Errorf("chunk source is required")
	}
	if analyzer == nil {
		return nil, fmt.Errorf("query analyzer is required")
	}

	abs, err := filepath.Abs(workspaceDir)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace dir: %w", err)
	}

	chunks, err := source.Collect(ctx, abs)
	if err != nil {
		return nil, err
	}

	return &Backend{
		workspaceDir:  abs,
		chunkSource:   source,
		queryAnalyzer: analyzer,
		index:         buildIndex(chunks),
	}, nil
}

// Name identifies the backend implementation.
func (b *Backend) Name() string {
	return "memory"
}

// Search executes a fuzzy query against the in-memory backend.
func (b *Backend) Search(ctx context.Context, query string, opts search.Options) ([]search.Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if b == nil {
		return nil, fmt.Errorf("search index not initialized")
	}

	b.mu.RLock()
	index := b.index
	analyzer := b.queryAnalyzer
	b.mu.RUnlock()

	if index == nil {
		return nil, fmt.Errorf("search index not initialized")
	}

	if analyzer == nil {
		return nil, fmt.Errorf("query analyzer not initialized")
	}

	queryTokens := analyzer.Analyze(query)
	if len(queryTokens) == 0 {
		return nil, nil
	}

	if opts.TopK <= 0 {
		opts.TopK = 10
	}

	scored := index.score(queryTokens)
	if len(scored) == 0 {
		return nil, nil
	}

	rerankResults(scored, queryTokens, opts)
	filtered := scored[:0]
	for _, item := range scored {
		if item.score <= 0 {
			continue
		}
		filtered = append(filtered, item)
	}
	scored = filtered
	if len(scored) == 0 {
		return nil, nil
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			if scored[i].entry.chunk.File == scored[j].entry.chunk.File {
				return scored[i].entry.chunk.Symbol < scored[j].entry.chunk.Symbol
			}
			return scored[i].entry.chunk.File < scored[j].entry.chunk.File
		}
		return scored[i].score > scored[j].score
	})

	if len(scored) > opts.TopK {
		scored = scored[:opts.TopK]
	}

	normalizeScores(scored)

	results := make([]search.Result, 0, len(scored))
	for _, item := range scored {
		chunk := item.entry.chunk
		results = append(results, search.Result{
			File:      chunk.File,
			Symbol:    chunk.Symbol,
			Kind:      chunk.Kind,
			StartLine: chunk.StartLine,
			EndLine:   chunk.EndLine,
			Score:     item.score,
			Summary:   chunk.Summary,
		})
	}
	return results, nil
}

// ApplyFileChanges refreshes the in-memory index after workspace changes.
// The current memory backend uses full rebuilds as the first incremental strategy.
func (b *Backend) ApplyFileChanges(ctx context.Context, _ []protocol.FileEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if b == nil {
		return fmt.Errorf("search index not initialized")
	}

	b.mu.RLock()
	source := b.chunkSource
	workspaceDir := b.workspaceDir
	b.mu.RUnlock()

	if source == nil {
		return fmt.Errorf("chunk source not initialized")
	}

	chunks, err := source.Collect(ctx, workspaceDir)
	if err != nil {
		return err
	}

	b.mu.Lock()
	b.index = buildIndex(chunks)
	b.mu.Unlock()
	return nil
}

// ChunkCount returns the number of chunks currently indexed in memory.
func (b *Backend) ChunkCount() int {
	if b == nil {
		return 0
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.index == nil {
		return 0
	}
	return len(b.index.entries)
}
