package client

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ForeverSRC/mcp-gopls-plus/pkg/lsp/protocol"
)

const (
	defaultCallTimeout = 45 * time.Second
	clientName         = "mcp-gopls"
	clientVersion      = "2.0.0-dev"
)

// Option configures the gopls client.
type Option func(*clientOptions)

type clientOptions struct {
	executable   string
	workspaceDir string
	logger       *slog.Logger
	callTimeout  time.Duration
}

// WithExecutable overrides the gopls binary path.
func WithExecutable(path string) Option {
	return func(cfg *clientOptions) {
		cfg.executable = path
	}
}

// WithWorkspaceDir sets the workspace root used for gopls initialization.
func WithWorkspaceDir(dir string) Option {
	return func(cfg *clientOptions) {
		cfg.workspaceDir = dir
	}
}

// WithLogger injects a custom slog logger.
func WithLogger(logger *slog.Logger) Option {
	return func(cfg *clientOptions) {
		cfg.logger = logger
	}
}

// WithCallTimeout configures the RPC timeout used for blocking requests.
func WithCallTimeout(timeout time.Duration) Option {
	return func(cfg *clientOptions) {
		if timeout > 0 {
			cfg.callTimeout = timeout
		}
	}
}

// GoplsClient implements the LSPClient interface using a managed gopls process.
type GoplsClient struct {
	cmd          *exec.Cmd
	transport    *protocol.Transport
	logger       *slog.Logger
	callTimeout  time.Duration
	callOverride func(context.Context, string, any) (*protocol.JSONRPCMessage, error)

	workspaceDir string
	workspaceURI string

	sendMu      sync.Mutex
	nextID      atomic.Int64
	closed      atomic.Bool
	initialized atomic.Bool

	diagnosticsMu       sync.RWMutex
	diagnosticsCache    map[string][]protocol.Diagnostic
	diagnosticsHandlers map[int64]DiagnosticsHandler
	handlerCounter      atomic.Int64

	openedDocs sync.Map

	readerCtx    context.Context
	readerCancel context.CancelFunc
	readerDone   chan struct{}

	pendingMu sync.Mutex
	pending   map[int64]chan rpcResponse

	diagnosticsWaiters map[string][]chan struct{}

	closeOnce sync.Once
}

type rpcResponse struct {
	msg *protocol.JSONRPCMessage
	err error
}

// NewGoplsClient starts a new gopls subprocess and wires it to the MCP bridge.
func NewGoplsClient(opts ...Option) (*GoplsClient, error) {
	cfg := clientOptions{
		callTimeout: defaultCallTimeout,
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.logger == nil {
		cfg.logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))
	}

	execPath, err := resolveGoplsExecutable(cfg.executable)
	if err != nil {
		return nil, fmt.Errorf("resolve gopls executable: %w", err)
	}

	workspaceDir, workspaceURI, err := resolveWorkspace(cfg.workspaceDir)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(execPath, "serve", "-rpc.trace", "-logfile=auto")
	cmd.Env = buildGoplsEnv(os.Environ())

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}

	go pipeLogs(cfg.logger.With("component", "gopls"), stderr)

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("start gopls: %w", err)
	}

	client := &GoplsClient{
		cmd:                 cmd,
		transport:           protocol.NewTransport(bufio.NewReader(stdout), bufio.NewWriter(stdin)),
		logger:              cfg.logger.With("component", "lsp_client"),
		callTimeout:         cfg.callTimeout,
		workspaceDir:        workspaceDir,
		workspaceURI:        workspaceURI,
		diagnosticsCache:    make(map[string][]protocol.Diagnostic),
		diagnosticsHandlers: make(map[int64]DiagnosticsHandler),
		pending:             make(map[int64]chan rpcResponse),
		diagnosticsWaiters:  make(map[string][]chan struct{}),
	}

	client.nextID.Store(1)
	client.closed.Store(false)

	client.readerCtx, client.readerCancel = context.WithCancel(context.Background())
	client.readerDone = make(chan struct{})
	go client.readerLoop()

	client.logger.Info("gopls client started",
		"exec", execPath,
		"workspace", workspaceDir,
	)
	return client, nil
}

// Initialize satisfies the LSPClient interface.
func (c *GoplsClient) Initialize(ctx context.Context) error {
	if c.initialized.Load() {
		return nil
	}

	initParams := map[string]any{
		"processId": nil,
		"clientInfo": map[string]any{
			"name":    clientName,
			"version": clientVersion,
		},
		"rootUri": c.workspaceURI,
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"synchronization": map[string]any{
					"dynamicRegistration": true,
					"didSave":             true,
				},
				"completion": map[string]any{
					"dynamicRegistration": true,
					"completionItem": map[string]any{
						"snippetSupport": true,
					},
				},
				"hover": map[string]any{
					"dynamicRegistration": true,
					"contentFormat":       []string{"markdown", "plaintext"},
				},
				"definition": map[string]any{
					"dynamicRegistration": true,
				},
				"references": map[string]any{
					"dynamicRegistration": true,
				},
				"documentSymbol": map[string]any{
					"dynamicRegistration":               true,
					"hierarchicalDocumentSymbolSupport": true,
				},
				"publishDiagnostics": map[string]any{
					"relatedInformation": true,
				},
			},
			"workspace": map[string]any{
				"applyEdit": true,
				"symbol": map[string]any{
					"dynamicRegistration": true,
				},
			},
		},
		"trace": "messages",
	}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		_, lastErr = c.call(ctx, "initialize", initParams)
		if lastErr == nil {
			c.initialized.Store(true)
			break
		}

		if !errors.Is(lastErr, context.DeadlineExceeded) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if lastErr != nil {
		return fmt.Errorf("initialize gopls: %w", lastErr)
	}

	if err := c.notify("initialized", map[string]any{}); err != nil {
		c.initialized.Store(false)
		return fmt.Errorf("send initialized notification: %w", err)
	}

	c.logger.Info("gopls initialized", "workspace", c.workspaceDir)
	return nil
}

// Shutdown gracefully shuts gopls down.
func (c *GoplsClient) Shutdown(ctx context.Context) error {
	if !c.initialized.Load() {
		return nil
	}
	if _, err := c.call(ctx, "shutdown", nil); err != nil {
		return fmt.Errorf("shutdown gopls: %w", err)
	}
	return nil
}

// Close terminates the gopls process.
func (c *GoplsClient) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	var errs []error

	c.closeOnce.Do(func() {
		c.closed.Store(true)

		if c.readerCancel != nil {
			c.readerCancel()
		}
		if c.readerDone != nil {
			<-c.readerDone
		}

		if c.initialized.Load() {
			if err := c.Shutdown(ctx); err != nil {
				errs = append(errs, err)
			}
			if err := c.notify("exit", map[string]any{}); err != nil {
				errs = append(errs, err)
			}
			c.initialized.Store(false)
		}

		if c.transport != nil {
			_ = c.transport.Close()
		}

		if c.cmd != nil && c.cmd.Process != nil {
			if err := c.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				errs = append(errs, fmt.Errorf("kill gopls: %w", err))
			}
			_, _ = c.cmd.Process.Wait()
		}
	})

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}

	return nil
}
