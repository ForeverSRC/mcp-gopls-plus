package server

import (
	"context"
	"fmt"

	"github.com/ForeverSRC/mcp-gopls-plus/pkg/fs"
	"github.com/ForeverSRC/mcp-gopls-plus/pkg/lsp/protocol"
	"github.com/ForeverSRC/mcp-gopls-plus/pkg/search"
)

type lspFSNotifier interface {
	NotifyDidChangeWatchedFiles(context.Context, []protocol.FileEvent) error
}

type combinedFSNotifier struct {
	lsp      lspFSNotifier
	searcher search.ChangeAwareSearcher
}

func (n combinedFSNotifier) NotifyDidChangeWatchedFiles(ctx context.Context, changes []protocol.FileEvent) error {
	if n.lsp != nil {
		if err := n.lsp.NotifyDidChangeWatchedFiles(ctx, changes); err != nil {
			return err
		}
	}
	if n.searcher != nil {
		if err := n.searcher.ApplyFileChanges(ctx, changes); err != nil {
			return err
		}
	}
	return nil
}

// NewService creates a fully configured MCP service ready to serve requests.
func NewService(cfg Config) (*Service, error) {
	if err := cfg.Normalize(); err != nil {
		return nil, err
	}

	logFile, logger, err := setupLogger(cfg)
	if err != nil {
		return nil, err
	}

	svc := &Service{
		config:  cfg,
		logger:  logger,
		logFile: logFile,
	}

	searcher, err := newSearcher(cfg.WorkspaceDir)
	if err != nil {
		svc.cleanup(context.Background())
		return nil, fmt.Errorf("build code search index: %w", err)
	}
	svc.searcher = searcher

	if err := svc.initLSPClient(context.Background()); err != nil {
		svc.cleanup(context.Background())
		return nil, fmt.Errorf("bootstrap lsp client: %w", err)
	}

	svc.server = setupServer(logger)
	svc.registerResources()
	svc.registerPrompts()

	if cfg.FSWatch {
		notifier, ok := svc.lspClient.(lspFSNotifier)
		if ok {
			var changeAware search.ChangeAwareSearcher
			if searcher, ok := svc.searcher.(search.ChangeAwareSearcher); ok {
				changeAware = searcher
			}
			watcher := fs.NewWatcher(cfg.WorkspaceDir, combinedFSNotifier{
				lsp:      notifier,
				searcher: changeAware,
			})
			watcher = watcher.WithLogger(logger.With("component", "fs_watcher"))
			svc.fsWatcher = watcher
		}
	}

	return svc, nil
}
