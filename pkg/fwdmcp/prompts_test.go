package fwdmcp

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHandleTroubleshootPrompt(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test with no arguments
	req := &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{
			Name:      "troubleshoot",
			Arguments: nil,
		},
	}

	result, err := server.handleTroubleshootPrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if result.Description == "" {
		t.Error("Expected non-empty description")
	}
	if len(result.Messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(result.Messages))
	}
	if result.Messages[0].Role != "user" {
		t.Errorf("Expected user role, got %s", result.Messages[0].Role)
	}

	// Verify content is non-empty
	textContent := result.Messages[0].Content.(*mcp.TextContent)
	if textContent.Text == "" {
		t.Error("Expected non-empty text content")
	}

	// Test with service_name argument
	req.Params.Arguments = map[string]string{
		"service_name": "my-service",
	}

	result, err = server.handleTroubleshootPrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	textContent = result.Messages[0].Content.(*mcp.TextContent)
	if !strings.Contains(textContent.Text, "my-service") {
		t.Error("Expected content to contain service name")
	}

	// Test with symptom argument
	req.Params.Arguments = map[string]string{
		"symptom": "connection refused",
	}

	result, err = server.handleTroubleshootPrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	textContent = result.Messages[0].Content.(*mcp.TextContent)
	if !strings.Contains(textContent.Text, "connection refused") {
		t.Error("Expected content to contain symptom")
	}

	// Test with both arguments
	req.Params.Arguments = map[string]string{
		"service_name": "my-service",
		"symptom":      "timeout",
	}

	result, err = server.handleTroubleshootPrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	textContent = result.Messages[0].Content.(*mcp.TextContent)
	if !strings.Contains(textContent.Text, "my-service") {
		t.Error("Expected content to contain service name")
	}
	if !strings.Contains(textContent.Text, "timeout") {
		t.Error("Expected content to contain symptom")
	}
}

func TestHandleExplainStatusPrompt(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	req := &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{
			Name:      "explain_status",
			Arguments: nil,
		},
	}

	result, err := server.handleExplainStatusPrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if result.Description == "" {
		t.Error("Expected non-empty description")
	}
	if len(result.Messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(result.Messages))
	}

	// Verify content includes expected guidance
	textContent := result.Messages[0].Content.(*mcp.TextContent)
	if !strings.Contains(textContent.Text, "not be familiar") {
		t.Error("Expected content to mention explaining to someone not familiar with Kubernetes")
	}
	if !strings.Contains(textContent.Text, "get_health") {
		t.Error("Expected content to reference get_health tool")
	}
}

func TestHandleFixErrorsPrompt(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test without auto_reconnect
	req := &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{
			Name:      "fix_errors",
			Arguments: nil,
		},
	}

	result, err := server.handleFixErrorsPrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	textContent := result.Messages[0].Content.(*mcp.TextContent)
	if !strings.Contains(textContent.Text, "Manual Reconnect") {
		t.Error("Expected content to contain Manual Reconnect section")
	}
	if strings.Contains(textContent.Text, "Auto-Reconnect Mode") {
		t.Error("Expected content NOT to contain Auto-Reconnect Mode when disabled")
	}

	// Test with auto_reconnect=false
	req.Params.Arguments = map[string]string{
		"auto_reconnect": "false",
	}

	result, err = server.handleFixErrorsPrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	textContent = result.Messages[0].Content.(*mcp.TextContent)
	if !strings.Contains(textContent.Text, "Manual Reconnect") {
		t.Error("Expected content to contain Manual Reconnect section")
	}

	// Test with auto_reconnect=true
	req.Params.Arguments = map[string]string{
		"auto_reconnect": "true",
	}

	result, err = server.handleFixErrorsPrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	textContent = result.Messages[0].Content.(*mcp.TextContent)
	if !strings.Contains(textContent.Text, "Auto-Reconnect Mode") {
		t.Error("Expected content to contain Auto-Reconnect Mode section")
	}
	if strings.Contains(textContent.Text, "Manual Reconnect") {
		t.Error("Expected content NOT to contain Manual Reconnect when auto_reconnect is true")
	}
}

func TestHandleMonitorPrompt(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	req := &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{
			Name:      "monitor",
			Arguments: nil,
		},
	}

	result, err := server.handleMonitorPrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if result.Description == "" {
		t.Error("Expected non-empty description")
	}
	if len(result.Messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(result.Messages))
	}

	// Verify content includes expected monitoring guidance
	textContent := result.Messages[0].Content.(*mcp.TextContent)
	if !strings.Contains(textContent.Text, "Health Check") {
		t.Error("Expected content to contain Health Check section")
	}
	if !strings.Contains(textContent.Text, "Traffic Analysis") {
		t.Error("Expected content to contain Traffic Analysis section")
	}
	if !strings.Contains(textContent.Text, "get_metrics") {
		t.Error("Expected content to reference get_metrics tool")
	}
}

func TestPromptResults(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	testCases := []struct {
		name        string
		handler     func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error)
		expectedStr string
	}{
		{
			name:        "troubleshoot",
			handler:     server.handleTroubleshootPrompt,
			expectedStr: "Check Service Status",
		},
		{
			name:        "explain_status",
			handler:     server.handleExplainStatusPrompt,
			expectedStr: "What kubefwd is doing",
		},
		{
			name:        "fix_errors",
			handler:     server.handleFixErrorsPrompt,
			expectedStr: "Assess the Situation",
		},
		{
			name:        "monitor",
			handler:     server.handleMonitorPrompt,
			expectedStr: "Service Overview",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &mcp.GetPromptRequest{
				Params: &mcp.GetPromptParams{
					Name: tc.name,
				},
			}

			result, err := tc.handler(context.Background(), req)
			if err != nil {
				t.Fatalf("Unexpected error for %s: %v", tc.name, err)
			}
			if result == nil {
				t.Fatalf("Expected non-nil result for %s", tc.name)
			}

			textContent := result.Messages[0].Content.(*mcp.TextContent)
			if !strings.Contains(textContent.Text, tc.expectedStr) {
				t.Errorf("Expected %s content to contain '%s'", tc.name, tc.expectedStr)
			}
		})
	}
}
