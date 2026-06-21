package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/hloiseau/mcp-gopls/v2/pkg/lsp/protocol"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (t *LSPTools) registerRefactorTools(s *server.MCPServer) {
	t.registerFormatDocument(s)
	t.registerRenameSymbol(s)
	t.registerCodeActionsTool(s)
}

func (t *LSPTools) registerFormatDocument(s *server.MCPServer) {
	tool := mcp.NewTool("format_document",
		mcp.WithDescription("Return formatting edits for a Go file"),
		mcp.WithTitleAnnotation("Format Document"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("file_uri",
			mcp.Required(),
			mcp.Description("URI of the file to format"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := getArguments(request)
		if err != nil {
			return nil, err
		}

		fileURI, err := getStringArg(args, "file_uri")
		if err != nil {
			return nil, err
		}

		if !strings.HasPrefix(fileURI, "file://") {
			fileURI = convertPathToURI(fileURI)
		}

		lspClient := t.getClient()
		if lspClient == nil {
			return nil, fmt.Errorf("LSP client not initialized")
		}

		edits, err := lspClient.DocumentFormatting(ctx, fileURI)
		if err != nil {
			return nil, t.handleLSPError(err)
		}

		result, err := mcp.NewToolResultJSON(map[string]any{
			"file_uri": fileURI,
			"edits":    edits,
		})
		if err != nil {
			return nil, err
		}
		return result, nil
	})
}

func (t *LSPTools) registerRenameSymbol(s *server.MCPServer) {
	tool := mcp.NewTool("rename_symbol",
		mcp.WithDescription("Compute rename edits for a symbol"),
		mcp.WithTitleAnnotation("Rename Symbol"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("file_uri",
			mcp.Required(),
			mcp.Description("URI of the file"),
		),
		mcp.WithNumber("line",
			mcp.Required(),
			mcp.Description("Line number (0-indexed)"),
		),
		mcp.WithNumber("character",
			mcp.Required(),
			mcp.Description("Character offset (0-indexed)"),
		),
		mcp.WithString("new_name",
			mcp.Required(),
			mcp.Description("New identifier name"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := getArguments(request)
		if err != nil {
			return nil, err
		}

		fileURI, err := getStringArg(args, "file_uri")
		if err != nil {
			return nil, err
		}

		line, err := getIntArg(args, "line")
		if err != nil {
			return nil, err
		}
		character, err := getIntArg(args, "character")
		if err != nil {
			return nil, err
		}

		newName, err := getStringArg(args, "new_name")
		if err != nil {
			return nil, err
		}

		if !strings.HasPrefix(fileURI, "file://") {
			fileURI = convertPathToURI(fileURI)
		}

		lspClient := t.getClient()
		if lspClient == nil {
			return nil, fmt.Errorf("LSP client not initialized")
		}

		edit, err := lspClient.Rename(ctx, fileURI, line, character, newName)
		if err != nil {
			return nil, t.handleLSPError(err)
		}

		payload := map[string]any{
			"file_uri": fileURI,
			"new_name": newName,
			"edits":    edit,
		}
		result, err := mcp.NewToolResultJSON(payload)
		if err != nil {
			return nil, err
		}
		return result, nil
	})
}

func (t *LSPTools) registerCodeActionsTool(s *server.MCPServer) {
	tool := mcp.NewTool("list_code_actions",
		mcp.WithDescription("List available code actions for a given range"),
		mcp.WithTitleAnnotation("List Code Actions"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("file_uri",
			mcp.Required(),
			mcp.Description("URI of the file"),
		),
		mcp.WithNumber("start_line",
			mcp.Required(),
			mcp.Description("Range start line (0-indexed)"),
		),
		mcp.WithNumber("start_character",
			mcp.Required(),
			mcp.Description("Range start character (0-indexed)"),
		),
		mcp.WithNumber("end_line",
			mcp.Required(),
			mcp.Description("Range end line (0-indexed)"),
		),
		mcp.WithNumber("end_character",
			mcp.Required(),
			mcp.Description("Range end character (0-indexed)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := getArguments(request)
		if err != nil {
			return nil, err
		}

		fileURI, err := getStringArg(args, "file_uri")
		if err != nil {
			return nil, err
		}

		startLine, err := getIntArg(args, "start_line")
		if err != nil {
			return nil, err
		}
		startChar, err := getIntArg(args, "start_character")
		if err != nil {
			return nil, err
		}
		endLine, err := getIntArg(args, "end_line")
		if err != nil {
			return nil, err
		}
		endChar, err := getIntArg(args, "end_character")
		if err != nil {
			return nil, err
		}
		rng := protocol.Range{
			Start: protocol.Position{Line: startLine, Character: startChar},
			End:   protocol.Position{Line: endLine, Character: endChar},
		}

		if !strings.HasPrefix(fileURI, "file://") {
			fileURI = convertPathToURI(fileURI)
		}

		lspClient := t.getClient()
		if lspClient == nil {
			return nil, fmt.Errorf("LSP client not initialized")
		}

		actions, err := lspClient.CodeActions(ctx, fileURI, rng)
		if err != nil {
			return nil, t.handleLSPError(err)
		}

		result, err := mcp.NewToolResultJSON(map[string]any{
			"file_uri": fileURI,
			"actions":  actions,
		})
		if err != nil {
			return nil, err
		}
		return result, nil
	})
}
