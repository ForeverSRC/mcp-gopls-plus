package fs_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ForeverSRC/mcp-gopls-plus/pkg/fs"
	"github.com/ForeverSRC/mcp-gopls-plus/pkg/lsp/protocol"
)

// stubNotifier captures calls to NotifyDidChangeWatchedFiles for assertions.
type stubNotifier struct {
	mu       sync.Mutex
	calls    [][]protocol.FileEvent
	notifyCh chan struct{}
}

func newStubNotifier() *stubNotifier {
	return &stubNotifier{notifyCh: make(chan struct{}, 16)}
}

func (s *stubNotifier) NotifyDidChangeWatchedFiles(_ context.Context, changes []protocol.FileEvent) error {
	s.mu.Lock()
	s.calls = append(s.calls, changes)
	s.mu.Unlock()
	select {
	case s.notifyCh <- struct{}{}:
	default:
	}
	return nil
}

func (s *stubNotifier) waitForNotification(t *testing.T, timeout time.Duration) {
	t.Helper()
	select {
	case <-s.notifyCh:
	case <-time.After(timeout):
		t.Fatal("timed out waiting for gopls notification")
	}
}

func (s *stubNotifier) allChanges() []protocol.FileEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []protocol.FileEvent
	for _, batch := range s.calls {
		out = append(out, batch...)
	}
	return out
}

// startWatcher launches Run in a goroutine and returns a cancel func.
// It waits briefly so the inotify watches are in place before callers write files.
func startWatcher(t *testing.T, dir string, n fs.Notifier) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	w := fs.NewWatcher(dir, n)
	go w.Run(ctx)
	time.Sleep(50 * time.Millisecond)
	return cancel
}

func TestWatcher_DetectsFileCreation(t *testing.T) {
	dir := t.TempDir()
	notifier := newStubNotifier()
	cancel := startWatcher(t, dir, notifier)
	defer cancel()

	path := filepath.Join(dir, "foo.go")
	if err := os.WriteFile(path, []byte("package foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	notifier.waitForNotification(t, 2*time.Second)

	changes := notifier.allChanges()
	if len(changes) == 0 {
		t.Fatal("expected at least one file event, got none")
	}
	if !strings.HasSuffix(changes[0].URI, "foo.go") {
		t.Errorf("unexpected URI %q", changes[0].URI)
	}
}

func TestWatcher_DetectsFileDeletion(t *testing.T) {
	dir := t.TempDir()

	// Pre-create the file before starting the watcher so Create noise is gone.
	path := filepath.Join(dir, "bar.go")
	if err := os.WriteFile(path, []byte("package bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	notifier := newStubNotifier()
	cancel := startWatcher(t, dir, notifier)
	defer cancel()

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	notifier.waitForNotification(t, 2*time.Second)

	for _, ev := range notifier.allChanges() {
		if strings.HasSuffix(ev.URI, "bar.go") && ev.Type == protocol.FileDeleted {
			return
		}
	}
	t.Errorf("expected FileDeleted event for bar.go, got: %v", notifier.allChanges())
}

func TestWatcher_IgnoresNonGoFiles(t *testing.T) {
	dir := t.TempDir()
	notifier := newStubNotifier()
	cancel := startWatcher(t, dir, notifier)
	defer cancel()

	// Write a non-Go file — should produce no notification.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Also write a real Go file to confirm the watcher is alive.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	notifier.waitForNotification(t, 2*time.Second)

	for _, ev := range notifier.allChanges() {
		if strings.HasSuffix(ev.URI, "README.md") {
			t.Errorf("README.md should not trigger a notification, got %v", ev)
		}
	}
}
