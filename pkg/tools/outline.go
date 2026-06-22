package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/ForeverSRC/mcp-gopls-plus/pkg/lsp/protocol"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (t *LSPTools) registerOutlineTool(s *server.MCPServer) {
	tool := mcp.NewTool("file_outline",
		mcp.WithDescription("List symbols (functions, types, methods) defined in a Go file. Returns name, kind, and line range for each."),
		mcp.WithTitleAnnotation("File Outline"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("file_uri",
			mcp.Required(),
			mcp.Description("URI of the file"),
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

		// Ensure the file is open in gopls so documentSymbol returns proper positions.
		if err := lspClient.DidOpen(ctx, fileURI, "go", ""); err != nil {
			return nil, fmt.Errorf("failed to open file for outline: %w", err)
		}

		symbols, err := lspClient.DocumentSymbols(ctx, fileURI)
		if err != nil {
			return nil, t.handleLSPError(err)
		}

		outline := flattenSymbols(symbols)

		result, err := mcp.NewToolResultJSON(map[string]any{
			"file_uri": fileURI,
			"symbols":  outline,
		})
		if err != nil {
			return nil, err
		}
		return result, nil
	})
}

// outlineSymbol is the output structure for a single symbol in the file outline.
type outlineSymbol struct {
	Name      string          `json:"name"`
	Kind      string          `json:"kind"`
	StartLine int             `json:"start_line"`
	EndLine   int             `json:"end_line"`
	Children  []outlineSymbol `json:"children,omitempty"`
}

// flattenSymbols converts DocumentSymbol tree into outlineSymbol tree.
func flattenSymbols(symbols []protocol.DocumentSymbol) []outlineSymbol {
	if len(symbols) == 0 {
		return nil
	}

	result := make([]outlineSymbol, 0, len(symbols))
	for _, sym := range symbols {
		item := outlineSymbol{
			Name:      sym.Name,
			Kind:      symbolKindString(sym.Kind),
			StartLine: sym.Range.Start.Line + 1, // 0-indexed from LSP -> 1-indexed for user
			EndLine:   sym.Range.End.Line + 1,
		}
		if len(sym.Children) > 0 {
			item.Children = flattenSymbols(sym.Children)
		}
		result = append(result, item)
	}
	return result
}

// symbolKindStrings maps LSP SymbolKind (1-indexed) to human-readable strings.
// Defined as a slice indexed by kind value; index 0 is unused so that kind 1 maps to symbolKindStrings[1].
// See https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#symbolKind
var symbolKindStrings = []string{
	0:  "", // 0 is not a valid SymbolKind
	1:  "file",
	2:  "module",
	3:  "namespace",
	4:  "package",
	5:  "class",
	6:  "method",
	7:  "property",
	8:  "field",
	9:  "constructor",
	10: "enum",
	11: "interface",
	12: "function",
	13: "variable",
	14: "constant",
	15: "string",
	16: "number",
	17: "boolean",
	18: "array",
	19: "object",
	20: "key",
	21: "null",
	22: "enum_member",
	23: "struct",
	24: "event",
	25: "operator",
	26: "type_parameter",
}

// symbolKindString maps LSP SymbolKind integer to a human-readable string.
func symbolKindString(kind int) string {
	if kind > 0 && kind < len(symbolKindStrings) {
		return symbolKindStrings[kind]
	}
	return fmt.Sprintf("unknown(%d)", kind)
}
