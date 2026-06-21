package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (t *LSPTools) registerNavigationTools(s *server.MCPServer) {
	t.registerGoToDefinition(s)
	t.registerFindReferences(s)
}

func (t *LSPTools) registerGoToDefinition(s *server.MCPServer) {
	definitionTool := mcp.NewTool("go_to_definition",
		mcp.WithDescription("Navigate to the definition of a symbol"),
		mcp.WithTitleAnnotation("Go To Definition"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("file_uri",
			mcp.Required(),
			mcp.Description("URI of the file"),
		),
		mcp.WithNumber("line",
			mcp.Required(),
			mcp.Description("Line number (1-indexed, first line = 1)"),
		),
		mcp.WithNumber("character",
			mcp.Required(),
			mcp.Description("Character offset (1-indexed, first character = 1)"),
		),
	)

	s.AddTool(definitionTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		if !strings.HasPrefix(fileURI, "file://") {
			fileURI = convertPathToURI(fileURI)
		}

		lspClient := t.getClient()
		if lspClient == nil {
			return nil, fmt.Errorf("LSP client not available")
		}

		// Convert 1-indexed (user-facing) to 0-indexed (LSP protocol)
		lspLine := line - 1
		lspChar := character - 1

		locations, err := lspClient.GoToDefinition(ctx, fileURI, lspLine, lspChar)
		if err != nil {
			if isPositionError(err) {
				return nil, fmt.Errorf("go_to_definition failed at (%d,%d): %w (Tip: line/character are 1-indexed)", line, character, err)
			}
			return nil, t.handleLSPError(err)
		}

		payload := map[string]any{
			"file_uri":  fileURI,
			"positions": locations,
		}
		result, err := mcp.NewToolResultJSON(payload)
		if err != nil {
			return nil, err
		}
		return result, nil
	})
}

func (t *LSPTools) registerFindReferences(s *server.MCPServer) {
	referencesTool := mcp.NewTool("find_references",
		mcp.WithDescription("Find all references to a symbol"),
		mcp.WithTitleAnnotation("Find References"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("file_uri",
			mcp.Required(),
			mcp.Description("URI of the file"),
		),
		mcp.WithNumber("line",
			mcp.Required(),
			mcp.Description("Line number (1-indexed, first line = 1)"),
		),
		mcp.WithNumber("character",
			mcp.Required(),
			mcp.Description("Character offset (1-indexed, first character = 1)"),
		),
	)

	s.AddTool(referencesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		if !strings.HasPrefix(fileURI, "file://") {
			fileURI = convertPathToURI(fileURI)
		}

		lspClient := t.getClient()
		if lspClient == nil {
			return nil, fmt.Errorf("LSP client not available")
		}

		// Convert 1-indexed (user-facing) to 0-indexed (LSP protocol)
		lspLine := line - 1
		lspChar := character - 1

		locations, err := lspClient.FindReferences(ctx, fileURI, lspLine, lspChar, true)
		if err != nil {
			if strings.Contains(err.Error(), "client closed") {
				return nil, fmt.Errorf("LSP client not available, please restart the server: %w", err)
			}
			if isPositionError(err) {
				return nil, fmt.Errorf("find_references failed at (%d,%d): %w (Tip: line/character are 1-indexed)", line, character, err)
			}
			return nil, t.handleLSPError(err)
		}

		payload := map[string]any{
			"file_uri":   fileURI,
			"references": locations,
		}
		result, err := mcp.NewToolResultJSON(payload)
		if err != nil {
			return nil, err
		}
		return result, nil
	})
}
