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

func TestHandleSetupLocalDevPrompt(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test with no arguments
	req := &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{
			Name:      "setup_local_dev",
			Arguments: nil,
		},
	}

	result, err := server.handleSetupLocalDevPrompt(context.Background(), req)
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

	textContent := result.Messages[0].Content.(*mcp.TextContent)
	if !strings.Contains(textContent.Text, "Discover Available Services") {
		t.Error("Expected content to contain 'Discover Available Services'")
	}

	// Test with namespace argument
	req.Params.Arguments = map[string]string{
		"namespace": "production",
	}

	result, err = server.handleSetupLocalDevPrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	textContent = result.Messages[0].Content.(*mcp.TextContent)
	if !strings.Contains(textContent.Text, "production") {
		t.Error("Expected content to contain namespace")
	}

	// Test with context argument
	req.Params.Arguments = map[string]string{
		"context": "my-cluster",
	}

	result, err = server.handleSetupLocalDevPrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	textContent = result.Messages[0].Content.(*mcp.TextContent)
	if !strings.Contains(textContent.Text, "my-cluster") {
		t.Error("Expected content to contain context")
	}

	// Test with both arguments
	req.Params.Arguments = map[string]string{
		"namespace": "staging",
		"context":   "dev-cluster",
	}

	result, err = server.handleSetupLocalDevPrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	textContent = result.Messages[0].Content.(*mcp.TextContent)
	if !strings.Contains(textContent.Text, "staging") {
		t.Error("Expected content to contain namespace")
	}
	if !strings.Contains(textContent.Text, "dev-cluster") {
		t.Error("Expected content to contain context")
	}
}

func TestHandleConnectionGuidePrompt(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test with service_name only
	req := &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{
			Name: "connection_guide",
			Arguments: map[string]string{
				"service_name": "postgres",
			},
		},
	}

	result, err := server.handleConnectionGuidePrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if !strings.Contains(result.Description, "postgres") {
		t.Error("Expected description to contain service name")
	}

	textContent := result.Messages[0].Content.(*mcp.TextContent)
	if !strings.Contains(textContent.Text, "postgres") {
		t.Error("Expected content to contain service name")
	}
	if !strings.Contains(textContent.Text, "Get Connection Details") {
		t.Error("Expected content to contain connection guidance")
	}

	// Test with language argument
	req.Params.Arguments = map[string]string{
		"service_name": "redis",
		"language":     "python",
	}

	result, err = server.handleConnectionGuidePrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	textContent = result.Messages[0].Content.(*mcp.TextContent)
	if !strings.Contains(textContent.Text, "redis") {
		t.Error("Expected content to contain service name")
	}
	if !strings.Contains(textContent.Text, "python") {
		t.Error("Expected content to contain language")
	}

	// Test with no arguments
	req.Params.Arguments = nil

	result, err = server.handleConnectionGuidePrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result even without arguments")
	}
}

func TestHandleForwardNamespacePrompt(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test with namespace only
	req := &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{
			Name: "forward_namespace",
			Arguments: map[string]string{
				"namespace": "default",
			},
		},
	}

	result, err := server.handleForwardNamespacePrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if !strings.Contains(result.Description, "default") {
		t.Error("Expected description to contain namespace")
	}

	textContent := result.Messages[0].Content.(*mcp.TextContent)
	if !strings.Contains(textContent.Text, "default") {
		t.Error("Expected content to contain namespace")
	}
	if !strings.Contains(textContent.Text, "Preview Services") {
		t.Error("Expected content to contain preview guidance")
	}
	if !strings.Contains(textContent.Text, "add_namespace") {
		t.Error("Expected content to reference add_namespace tool")
	}

	// Test with selector argument
	req.Params.Arguments = map[string]string{
		"namespace": "production",
		"selector":  "app=api",
	}

	result, err = server.handleForwardNamespacePrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	textContent = result.Messages[0].Content.(*mcp.TextContent)
	if !strings.Contains(textContent.Text, "production") {
		t.Error("Expected content to contain namespace")
	}
	if !strings.Contains(textContent.Text, "app=api") {
		t.Error("Expected content to contain selector")
	}

	// Test with no arguments
	req.Params.Arguments = nil

	result, err = server.handleForwardNamespacePrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result even without arguments")
	}
}

func TestHandleDebugServicePrompt(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test with service_name only
	req := &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{
			Name: "debug_service",
			Arguments: map[string]string{
				"service_name": "api-gateway",
			},
		},
	}

	result, err := server.handleDebugServicePrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if !strings.Contains(result.Description, "api-gateway") {
		t.Error("Expected description to contain service name")
	}

	textContent := result.Messages[0].Content.(*mcp.TextContent)
	if !strings.Contains(textContent.Text, "api-gateway") {
		t.Error("Expected content to contain service name")
	}
	if !strings.Contains(textContent.Text, "Check Service Status") {
		t.Error("Expected content to contain debugging steps")
	}
	if !strings.Contains(textContent.Text, "Analyze HTTP Traffic") {
		t.Error("Expected content to contain HTTP traffic analysis")
	}

	// Test with symptom argument
	req.Params.Arguments = map[string]string{
		"service_name": "database",
		"symptom":      "500 errors",
	}

	result, err = server.handleDebugServicePrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	textContent = result.Messages[0].Content.(*mcp.TextContent)
	if !strings.Contains(textContent.Text, "database") {
		t.Error("Expected content to contain service name")
	}
	if !strings.Contains(textContent.Text, "500 errors") {
		t.Error("Expected content to contain symptom")
	}

	// Test with no arguments
	req.Params.Arguments = nil

	result, err = server.handleDebugServicePrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result even without arguments")
	}
}

func TestHandleQuickConnectPrompt(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test with service_name only
	req := &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{
			Name: "quick_connect",
			Arguments: map[string]string{
				"service_name": "redis",
			},
		},
	}

	result, err := server.handleQuickConnectPrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if !strings.Contains(result.Description, "redis") {
		t.Error("Expected description to contain service name")
	}

	textContent := result.Messages[0].Content.(*mcp.TextContent)
	if !strings.Contains(textContent.Text, "redis") {
		t.Error("Expected content to contain service name")
	}
	if !strings.Contains(textContent.Text, "Quick Connect Workflow") {
		t.Error("Expected content to contain workflow guidance")
	}
	if !strings.Contains(textContent.Text, "get_connection_info") {
		t.Error("Expected content to reference get_connection_info tool")
	}

	// Test with namespace argument
	req.Params.Arguments = map[string]string{
		"service_name": "postgres",
		"namespace":    "staging",
	}

	result, err = server.handleQuickConnectPrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	textContent = result.Messages[0].Content.(*mcp.TextContent)
	if !strings.Contains(textContent.Text, "postgres") {
		t.Error("Expected content to contain service name")
	}
	if !strings.Contains(textContent.Text, "staging") {
		t.Error("Expected content to contain namespace")
	}

	// Test with no arguments
	req.Params.Arguments = nil

	result, err = server.handleQuickConnectPrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result even without arguments")
	}
}

func TestHandleAnalyzeIssuesPrompt(t *testing.T) {
	resetGlobalState()
	server := Init("1.0.0")

	// Test with no arguments (this prompt takes no args)
	req := &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{
			Name:      "analyze_issues",
			Arguments: nil,
		},
	}

	result, err := server.handleAnalyzeIssuesPrompt(context.Background(), req)
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

	textContent := result.Messages[0].Content.(*mcp.TextContent)
	if !strings.Contains(textContent.Text, "Get Full Analysis") {
		t.Error("Expected content to contain analysis guidance")
	}
	if !strings.Contains(textContent.Text, "Prioritize Issues") {
		t.Error("Expected content to contain prioritization guidance")
	}
	if !strings.Contains(textContent.Text, "CRITICAL") {
		t.Error("Expected content to contain severity levels")
	}
	if !strings.Contains(textContent.Text, "get_analysis") {
		t.Error("Expected content to reference get_analysis tool")
	}
	if !strings.Contains(textContent.Text, "Resolution Plan") {
		t.Error("Expected content to contain resolution plan")
	}
}
