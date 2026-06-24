package search

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Chunk is the smallest searchable code unit stored in the index.
type Chunk struct {
	File         string
	Symbol       string
	Kind         string
	StartLine    int
	EndLine      int
	Summary      string
	Tokens       []string
	SymbolTokens []string
	IsTest       bool
	IsNoise      bool
}

// CollectWorkspaceChunks walks the workspace and builds AST-aware searchable chunks.
func CollectWorkspaceChunks(ctx context.Context, workspaceDir string) ([]Chunk, error) {
	var chunks []Chunk

	err := filepath.WalkDir(workspaceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		name := d.Name()
		if d.IsDir() {
			if shouldSkipDir(name) {
				return filepath.SkipDir
			}
			return nil
		}

		if !shouldIndexFile(name) {
			return nil
		}

		fileChunks, chunkErr := collectFileChunks(workspaceDir, path)
		if chunkErr != nil {
			return chunkErr
		}
		chunks = append(chunks, fileChunks...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk workspace: %w", err)
	}
	return chunks, nil
}

func shouldSkipDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata"
}

func shouldIndexFile(name string) bool {
	return strings.HasSuffix(name, ".go") || name == "go.mod" || name == "go.sum"
}

func collectFileChunks(workspaceDir, path string) ([]Chunk, error) {
	if strings.HasSuffix(path, ".go") {
		return collectGoFileChunks(workspaceDir, path)
	}
	return collectTextFileChunk(workspaceDir, path)
}

func collectGoFileChunks(workspaceDir, path string) ([]Chunk, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse go file %s: %w", path, err)
	}

	rel, err := filepath.Rel(workspaceDir, path)
	if err != nil {
		return nil, err
	}
	rel = filepath.ToSlash(rel)

	var chunks []Chunk
	for _, decl := range file.Decls {
		switch node := decl.(type) {
		case *ast.FuncDecl:
			chunks = append(chunks, buildFuncChunk(fset, rel, node))
		case *ast.GenDecl:
			chunks = append(chunks, buildGenDeclChunks(fset, rel, node)...)
		}
	}
	return chunks, nil
}

func buildFuncChunk(fset *token.FileSet, rel string, decl *ast.FuncDecl) Chunk {
	kind := "function"
	if decl.Recv != nil {
		kind = "method"
	}

	symbol := decl.Name.Name
	if decl.Recv != nil {
		symbol = formatMethodSymbol(decl)
	}

	summary := buildSummary(decl.Doc, decl)
	tokens := buildChunkTokens(rel, symbol, summary, renderNode(decl.Type))

	start := fset.Position(decl.Pos()).Line
	end := fset.Position(decl.End()).Line

	return Chunk{
		File:         rel,
		Symbol:       symbol,
		Kind:         kind,
		StartLine:    start,
		EndLine:      end,
		Summary:      summary,
		Tokens:       tokens,
		SymbolTokens: tokenizeIdentifier(symbol),
		IsTest:       strings.HasSuffix(rel, "_test.go"),
		IsNoise:      isNoiseFile(rel),
	}
}

func buildGenDeclChunks(fset *token.FileSet, rel string, decl *ast.GenDecl) []Chunk {
	var chunks []Chunk

	for _, spec := range decl.Specs {
		switch item := spec.(type) {
		case *ast.TypeSpec:
			summary := buildSummary(firstCommentGroup(item.Doc, decl.Doc), item)
			chunks = append(chunks, buildSpecChunk(fset, rel, item.Name.Name, "type", item, summary))
		case *ast.ValueSpec:
			kind := "var"
			if decl.Tok == token.CONST {
				kind = "const"
			}
			comment := firstCommentGroup(item.Doc, decl.Doc)
			summary := buildSummary(comment, item)
			for _, name := range item.Names {
				chunks = append(chunks, buildSpecChunk(fset, rel, name.Name, kind, item, summary))
			}
		}
	}

	return chunks
}

func buildSpecChunk(fset *token.FileSet, rel, symbol, kind string, node ast.Node, summary string) Chunk {
	start := fset.Position(node.Pos()).Line
	end := fset.Position(node.End()).Line

	signature := renderNode(node)
	if kind == "const" || kind == "var" {
		signature = kind + " " + symbol
	}

	return Chunk{
		File:         rel,
		Symbol:       symbol,
		Kind:         kind,
		StartLine:    start,
		EndLine:      end,
		Summary:      summary,
		Tokens:       buildChunkTokens(rel, symbol, summary, signature),
		SymbolTokens: tokenizeIdentifier(symbol),
		IsTest:       strings.HasSuffix(rel, "_test.go"),
		IsNoise:      isNoiseFile(rel),
	}
}

func collectTextFileChunk(workspaceDir, path string) ([]Chunk, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	rel, err := filepath.Rel(workspaceDir, path)
	if err != nil {
		return nil, err
	}
	rel = filepath.ToSlash(rel)

	summary := fmt.Sprintf("%s workspace file", filepath.Base(rel))
	content := string(data)
	lines := strings.Count(content, "\n") + 1
	if len(data) == 0 {
		lines = 1
	}

	return []Chunk{{
		File:         rel,
		Symbol:       filepath.Base(rel),
		Kind:         "file",
		StartLine:    1,
		EndLine:      lines,
		Summary:      summary,
		Tokens:       buildChunkTokens(rel, filepath.Base(rel), summary, content),
		SymbolTokens: tokenizeIdentifier(filepath.Base(rel)),
		IsNoise:      isNoiseFile(rel),
	}}, nil
}

func formatMethodSymbol(decl *ast.FuncDecl) string {
	if decl.Recv == nil || len(decl.Recv.List) == 0 {
		return decl.Name.Name
	}
	return receiverTypeName(decl.Recv.List[0].Type) + "." + decl.Name.Name
}

func receiverTypeName(expr ast.Expr) string {
	switch node := expr.(type) {
	case *ast.Ident:
		return node.Name
	case *ast.StarExpr:
		return "(*" + receiverTypeName(node.X) + ")"
	case *ast.IndexExpr:
		return receiverTypeName(node.X)
	case *ast.IndexListExpr:
		return receiverTypeName(node.X)
	case *ast.SelectorExpr:
		return renderNode(node)
	default:
		return renderNode(node)
	}
}

func buildSummary(doc *ast.CommentGroup, node ast.Node) string {
	if text := cleanComment(doc); text != "" {
		return text
	}
	return renderNode(node)
}

func cleanComment(doc *ast.CommentGroup) string {
	if doc == nil {
		return ""
	}
	text := strings.TrimSpace(doc.Text())
	if text == "" {
		return ""
	}
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 220 {
		text = text[:220]
		text = strings.TrimSpace(text) + "..."
	}
	return text
}

func buildChunkTokens(parts ...string) []string {
	var tokens []string
	for _, part := range parts {
		tokens = append(tokens, tokenizeText(part)...)
	}
	return tokens
}

func renderNode(node ast.Node) string {
	if node == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, token.NewFileSet(), node); err != nil {
		return ""
	}
	return strings.Join(strings.Fields(buf.String()), " ")
}

func firstCommentGroup(groups ...*ast.CommentGroup) *ast.CommentGroup {
	for _, group := range groups {
		if group != nil && strings.TrimSpace(group.Text()) != "" {
			return group
		}
	}
	return nil
}

func isNoiseFile(rel string) bool {
	lower := strings.ToLower(rel)
	return strings.Contains(lower, "/mock/") ||
		strings.Contains(lower, "/compat/") ||
		strings.Contains(lower, "/legacy/") ||
		strings.HasSuffix(lower, ".pb.go") ||
		strings.HasSuffix(lower, "_mock.go")
}
