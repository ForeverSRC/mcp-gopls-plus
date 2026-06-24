package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ForeverSRC/mcp-gopls-plus/pkg/lsp/protocol"
	"github.com/ForeverSRC/mcp-gopls-plus/pkg/search"
)

func TestBuildAndSearch(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "go.mod"), "module example.com/searchtest\n\ngo 1.25.0\n")
	writeFile(t, filepath.Join(tmp, "pkg", "auth", "handler.go"), `package auth

// HandleLogin authenticates the user and returns a JWT.
func HandleLogin(username string, password string) string {
	return username + password
}

// ParseConfig parses configuration values.
func ParseConfig(input string) string {
	return input
}
`)
	writeFile(t, filepath.Join(tmp, "pkg", "auth", "handler_test.go"), `package auth

func TestHandleLogin(t any) {}
`)

	backend, err := Build(context.Background(), tmp)
	if err != nil {
		t.Fatalf("build memory backend: %v", err)
	}
	if backend.Name() != "memory" {
		t.Fatalf("unexpected backend name: %s", backend.Name())
	}
	if backend.ChunkCount() == 0 {
		t.Fatal("expected chunks to be indexed")
	}

	results, err := backend.Search(context.Background(), "authentication handling", search.Options{TopK: 5})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].Symbol != "HandleLogin" {
		t.Fatalf("unexpected top result: %#v", results[0])
	}
	if results[0].Summary != "HandleLogin(username string, password string) string" &&
		results[0].Summary != "HandleLogin authenticates the user and returns a JWT." {
		t.Fatalf("unexpected summary: %q", results[0].Summary)
	}

	results, err = backend.Search(context.Background(), "test handle login", search.Options{TopK: 5})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	for _, result := range results {
		if result.File == "pkg/auth/handler_test.go" {
			t.Fatalf("unexpected test file result when include_tests=false: %#v", result)
		}
	}

	results, err = backend.Search(context.Background(), "test handle login", search.Options{TopK: 5, IncludeTests: true})
	if err != nil {
		t.Fatalf("search with tests failed: %v", err)
	}
	foundTest := false
	for _, result := range results {
		if result.File == "pkg/auth/handler_test.go" {
			foundTest = true
			break
		}
	}
	if !foundTest {
		t.Fatal("expected test file result when include_tests=true")
	}
}

func TestBuildWithSourceRequiresChunkSource(t *testing.T) {
	_, err := BuildWithSource(context.Background(), t.TempDir(), nil)
	if err == nil || err.Error() != "chunk source is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildWithSourceAndAnalyzerRequiresQueryAnalyzer(t *testing.T) {
	_, err := BuildWithSourceAndAnalyzer(context.Background(), t.TempDir(), search.WorkspaceChunkSource{}, nil)
	if err == nil || err.Error() != "query analyzer is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildWithSourceAndAnalyzerUsesInjectedAnalyzer(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "go.mod"), "module example.com/searchtest\n\ngo 1.25.0\n")
	writeFile(t, filepath.Join(tmp, "pkg", "auth", "handler.go"), `package auth

func HandleLogin(username string, password string) string {
	return username + password
}
`)

	backend, err := BuildWithSourceAndAnalyzer(
		context.Background(),
		tmp,
		search.WorkspaceChunkSource{},
		staticAnalyzer{tokens: []string{"handle", "login"}},
	)
	if err != nil {
		t.Fatalf("build memory backend with analyzer: %v", err)
	}

	results, err := backend.Search(context.Background(), "unrelated query", search.Options{TopK: 5})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) == 0 || results[0].Symbol != "HandleLogin" {
		t.Fatalf("expected injected analyzer to drive recall, got %#v", results)
	}
}

func TestApplyFileChangesRebuildsIndex(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "go.mod"), "module example.com/searchtest\n\ngo 1.25.0\n")
	target := filepath.Join(tmp, "pkg", "auth", "handler.go")
	writeFile(t, target, `package auth

func HandleLogin(username string, password string) string {
	return username + password
}
`)

	backend, err := Build(context.Background(), tmp)
	if err != nil {
		t.Fatalf("build memory backend: %v", err)
	}

	results, err := backend.Search(context.Background(), "parse config", search.Options{TopK: 5})
	if err != nil {
		t.Fatalf("search before rebuild failed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no ParseConfig result before rebuild, got %#v", results)
	}

	writeFile(t, target, `package auth

func HandleLogin(username string, password string) string {
	return username + password
}

func ParseConfig(input string) string {
	return input
}
`)

	err = backend.ApplyFileChanges(context.Background(), []protocol.FileEvent{{
		URI:  "file://" + filepath.ToSlash(target),
		Type: protocol.FileChanged,
	}})
	if err != nil {
		t.Fatalf("apply file changes failed: %v", err)
	}

	results, err = backend.Search(context.Background(), "parse config", search.Options{TopK: 5})
	if err != nil {
		t.Fatalf("search after rebuild failed: %v", err)
	}
	if len(results) == 0 || results[0].Symbol != "ParseConfig" {
		t.Fatalf("expected ParseConfig after rebuild, got %#v", results)
	}
}

type staticAnalyzer struct {
	tokens []string
}

func (a staticAnalyzer) Analyze(string) []string {
	return a.tokens
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
