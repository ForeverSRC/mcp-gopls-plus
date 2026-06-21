package server

import (
	"context"
	"fmt"

	"github.com/ForeverSRC/mcp-gopls-plus/pkg/fs"
	"github.com/ForeverSRC/mcp-gopls-plus/pkg/lsp/protocol"
)

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

	if err := svc.initLSPClient(context.Background()); err != nil {
		svc.cleanup(context.Background())
		return nil, fmt.Errorf("bootstrap lsp client: %w", err)
	}

	svc.server = setupServer(logger)
	svc.registerResources()
	svc.registerPrompts()

	if cfg.FSWatch {
		notifier, ok := svc.lspClient.(interface {
			NotifyDidChangeWatchedFiles(context.Context, []protocol.FileEvent) error
		})
		if ok {
			watcher := fs.NewWatcher(cfg.WorkspaceDir, notifier)
			watcher = watcher.WithLogger(logger.With("component", "fs_watcher"))
			svc.fsWatcher = watcher
		}
	}

	return svc, nil
}
