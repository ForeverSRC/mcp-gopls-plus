package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (t *LSPTools) registerInsightTools(s *server.MCPServer) {
	t.registerHover(s)
	t.registerCompletion(s)
}

func (t *LSPTools) registerHover(s *server.MCPServer) {
	hoverTool := mcp.NewTool("get_hover_info",
		mcp.WithDescription("Get hover information for a symbol"),
		mcp.WithTitleAnnotation("Get Hover Info"),
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

	s.AddTool(hoverTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
			return nil, fmt.Errorf("LSP client not initialized")
		}

		// Convert 1-indexed (user-facing) to 0-indexed (LSP protocol)
		lspLine := line - 1
		lspChar := character - 1

		info, err := lspClient.GetHover(ctx, fileURI, lspLine, lspChar)
		if err != nil {
			if strings.Contains(err.Error(), "client closed") {
				return nil, fmt.Errorf("LSP service not available, please restart the server: %w", err)
			}
			if isPositionError(err) {
				return nil, fmt.Errorf("get_hover_info failed at (%d,%d): %w (Tip: line/character are 1-indexed)", line, character, err)
			}
			return nil, fmt.Errorf("failed to get hover info: %w", err)
		}

		result, err := mcp.NewToolResultJSON(map[string]any{
			"file_uri": fileURI,
			"hover":    info,
		})
		if err != nil {
			return nil, err
		}
		return result, nil
	})
}

func (t *LSPTools) registerCompletion(s *server.MCPServer) {
	completionTool := mcp.NewTool("get_completion",
		mcp.WithDescription("Get completion suggestions at a position"),
		mcp.WithTitleAnnotation("Get Completion"),
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

	s.AddTool(completionTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
			return nil, fmt.Errorf("LSP client not initialized")
		}

		// Convert 1-indexed (user-facing) to 0-indexed (LSP protocol)
		lspLine := line - 1
		lspChar := character - 1

		completions, err := lspClient.GetCompletion(ctx, fileURI, lspLine, lspChar)
		if err != nil {
			if strings.Contains(err.Error(), "client closed") {
				return nil, fmt.Errorf("LSP service not available, please restart the server: %w", err)
			}
			if isPositionError(err) {
				return nil, fmt.Errorf("get_completion failed at (%d,%d): %w (Tip: line/character are 1-indexed)", line, character, err)
			}
			return nil, fmt.Errorf("failed to get completions: %w", err)
		}

		result, err := mcp.NewToolResultJSON(map[string]any{
			"file_uri":    fileURI,
			"completions": completions,
		})
		if err != nil {
			return nil, err
		}
		return result, nil
	})
}
