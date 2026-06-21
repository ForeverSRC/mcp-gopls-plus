package tools

import (
	"errors"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestGetArguments(t *testing.T) {
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]any{"key": "value"},
		},
	}
	args, err := getArguments(request)
	if err != nil {
		t.Fatalf("getArguments returned error: %v", err)
	}
	if args["key"] != "value" {
		t.Fatalf("unexpected argument value: %#v", args["key"])
	}
}

func TestCommandFailureResultIncludesStderr(t *testing.T) {
	tools := NewLSPTools(nil, ".")
	cmdResult := commandResult{
		Command:  []string{"go", "test", "./..."},
		ExitCode: 2,
		Stderr:   "line 1 failed\nline 2 detail",
	}

	result, err := tools.commandFailureResult("", cmdResult, errors.New("exit status 2"))
	if err != nil {
		t.Fatalf("commandFailureResult returned error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error result")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected text content")
	}

	textContent, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}

	if !strings.Contains(textContent.Text, "line 1 failed") {
		t.Fatalf("stderr not included in error text: %q", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "exit code 2") {
		t.Fatalf("exit code missing from error text: %q", textContent.Text)
	}
}
