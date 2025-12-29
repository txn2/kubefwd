//go:build live

// Package mcp provides a test client for the kubefwd MCP/API integration tests.
package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/txn2/kubefwd/pkg/fwdapi/types"
)

// Client is an HTTP client for the kubefwd REST API.
// It provides methods to call MCP-related endpoints for integration testing.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient creates a new MCP test client.
func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Response wraps API responses for easier testing.
type Response struct {
	Success bool             `json:"success"`
	Data    json.RawMessage  `json:"data,omitempty"`
	Error   *types.ErrorInfo `json:"error,omitempty"`
}

// doRequest performs an HTTP request and returns the response.
func (c *Client) doRequest(t *testing.T, method, path string, body interface{}) *Response {
	t.Helper()

	var reqBody io.Reader
	if body != nil {
		jsonBytes, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("Failed to marshal request body: %v", err)
		}
		reqBody = bytes.NewReader(jsonBytes)
	}

	req, err := http.NewRequest(method, c.BaseURL+path, reqBody)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	var result Response
	if err := json.Unmarshal(respBody, &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v (body: %s)", err, string(respBody))
	}

	return &result
}

// === Service Operations ===

// ListServices returns all forwarded services, optionally filtered by namespace.
func (c *Client) ListServices(t *testing.T, namespace string) []types.ServiceResponse {
	t.Helper()

	path := "/v1/services"
	if namespace != "" {
		path += "?namespace=" + url.QueryEscape(namespace)
	}

	resp := c.doRequest(t, "GET", path, nil)
	if !resp.Success {
		t.Fatalf("ListServices failed: %s", resp.Error.Message)
	}

	var result types.ServiceListResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatalf("Failed to unmarshal services: %v", err)
	}

	return result.Services
}

// GetService returns a specific service by key.
func (c *Client) GetService(t *testing.T, key string) *types.ServiceResponse {
	t.Helper()

	resp := c.doRequest(t, "GET", "/v1/services/"+url.PathEscape(key), nil)
	if !resp.Success {
		if resp.Error != nil && resp.Error.Code == "not_found" {
			return nil
		}
		t.Fatalf("GetService failed: %s", resp.Error.Message)
	}

	var result types.ServiceResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatalf("Failed to unmarshal service: %v", err)
	}

	return &result
}

// FindServices searches for services by query, port, or namespace.
func (c *Client) FindServices(t *testing.T, query string, port int, namespace string) []types.ServiceResponse {
	t.Helper()

	params := url.Values{}
	if query != "" {
		params.Set("query", query)
	}
	if port > 0 {
		params.Set("port", fmt.Sprintf("%d", port))
	}
	if namespace != "" {
		params.Set("namespace", namespace)
	}

	path := "/v1/services/find"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	resp := c.doRequest(t, "GET", path, nil)
	if !resp.Success {
		t.Fatalf("FindServices failed: %s", resp.Error.Message)
	}

	var result []types.ServiceResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatalf("Failed to unmarshal services: %v", err)
	}

	return result
}

// === Namespace Operations ===

// AddNamespaceResult is the result of adding a namespace.
type AddNamespaceResult struct {
	Success   bool
	Key       string
	Namespace string
	Context   string
	Services  []string
	Error     string
}

// AddNamespace starts forwarding all services in a namespace.
func (c *Client) AddNamespace(t *testing.T, namespace, context string) *AddNamespaceResult {
	t.Helper()

	req := types.AddNamespaceRequest{
		Namespace: namespace,
		Context:   context,
	}

	resp := c.doRequest(t, "POST", "/v1/namespace/add", req)

	result := &AddNamespaceResult{
		Success:   resp.Success,
		Namespace: namespace,
	}

	if !resp.Success {
		if resp.Error != nil {
			result.Error = resp.Error.Message
		}
		return result
	}

	var data types.AddNamespaceResponse
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal add namespace response: %v", err)
	}

	result.Key = data.Key
	result.Context = data.Context
	result.Services = data.Services

	return result
}

// RemoveNamespaceResult is the result of removing a namespace.
type RemoveNamespaceResult struct {
	Success   bool
	Namespace string
	Context   string
	Error     string
}

// RemoveNamespace stops forwarding all services in a namespace.
func (c *Client) RemoveNamespace(t *testing.T, namespace, context string) *RemoveNamespaceResult {
	t.Helper()

	path := "/v1/namespace/" + url.PathEscape(namespace)
	if context != "" {
		path += "?context=" + url.QueryEscape(context)
	}

	resp := c.doRequest(t, "DELETE", path, nil)

	result := &RemoveNamespaceResult{
		Success:   resp.Success,
		Namespace: namespace,
		Context:   context,
	}

	if !resp.Success && resp.Error != nil {
		result.Error = resp.Error.Message
	}

	return result
}

// === Service CRUD Operations ===

// AddService starts forwarding a specific service.
func (c *Client) AddService(t *testing.T, namespace, serviceName, context string) *types.AddServiceResponse {
	t.Helper()

	req := types.AddServiceRequest{
		Namespace:   namespace,
		ServiceName: serviceName,
		Context:     context,
	}

	resp := c.doRequest(t, "POST", "/v1/service/add", req)
	if !resp.Success {
		t.Fatalf("AddService failed: %s", resp.Error.Message)
	}

	var result types.AddServiceResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatalf("Failed to unmarshal add service response: %v", err)
	}

	return &result
}

// RemoveService stops forwarding a specific service.
func (c *Client) RemoveService(t *testing.T, key string) bool {
	t.Helper()

	resp := c.doRequest(t, "DELETE", "/v1/service/"+url.PathEscape(key), nil)
	return resp.Success
}

// === Connection Info ===

// GetConnectionInfo returns connection information for a forwarded service.
func (c *Client) GetConnectionInfo(t *testing.T, serviceName, namespace, context string) *types.ConnectionInfoResponse {
	t.Helper()

	params := url.Values{}
	if namespace != "" {
		params.Set("namespace", namespace)
	}
	if context != "" {
		params.Set("context", context)
	}

	path := "/v1/connection-info/" + url.PathEscape(serviceName)
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	resp := c.doRequest(t, "GET", path, nil)
	if !resp.Success {
		if resp.Error != nil && resp.Error.Code == "not_found" {
			return nil
		}
		t.Fatalf("GetConnectionInfo failed: %s", resp.Error.Message)
	}

	var result types.ConnectionInfoResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatalf("Failed to unmarshal connection info: %v", err)
	}

	return &result
}

// === Health and Status ===

// GetHealth returns the health status.
func (c *Client) GetHealth(t *testing.T) *types.HealthResponse {
	t.Helper()

	resp := c.doRequest(t, "GET", "/v1/health", nil)
	if !resp.Success {
		t.Fatalf("GetHealth failed: %s", resp.Error.Message)
	}

	var result types.HealthResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatalf("Failed to unmarshal health: %v", err)
	}

	return &result
}

// QuickStatus is the quick status response structure.
type QuickStatus struct {
	Status       string `json:"status"`
	Message      string `json:"message"`
	ErrorCount   int    `json:"errorCount"`
	ServiceCount int    `json:"serviceCount"`
}

// GetQuickStatus returns a quick status check.
func (c *Client) GetQuickStatus(t *testing.T) *QuickStatus {
	t.Helper()

	resp := c.doRequest(t, "GET", "/v1/status/quick", nil)
	if !resp.Success {
		t.Fatalf("GetQuickStatus failed: %s", resp.Error.Message)
	}

	var result QuickStatus
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatalf("Failed to unmarshal quick status: %v", err)
	}

	return &result
}

// === Kubernetes Discovery ===

// ListK8sNamespaces returns available Kubernetes namespaces.
func (c *Client) ListK8sNamespaces(t *testing.T, context string) []types.K8sNamespace {
	t.Helper()

	path := "/v1/kubernetes/namespaces"
	if context != "" {
		path += "?context=" + url.QueryEscape(context)
	}

	resp := c.doRequest(t, "GET", path, nil)
	if !resp.Success {
		t.Fatalf("ListK8sNamespaces failed: %s", resp.Error.Message)
	}

	var result types.K8sNamespacesResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatalf("Failed to unmarshal namespaces: %v", err)
	}

	return result.Namespaces
}

// ListK8sServices returns available Kubernetes services in a namespace.
func (c *Client) ListK8sServices(t *testing.T, namespace, context string) []types.K8sService {
	t.Helper()

	params := url.Values{}
	params.Set("namespace", namespace)
	if context != "" {
		params.Set("context", context)
	}

	resp := c.doRequest(t, "GET", "/v1/kubernetes/services?"+params.Encode(), nil)
	if !resp.Success {
		t.Fatalf("ListK8sServices failed: %s", resp.Error.Message)
	}

	var result types.K8sServicesResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatalf("Failed to unmarshal services: %v", err)
	}

	return result.Services
}

// ListContexts returns available Kubernetes contexts.
func (c *Client) ListContexts(t *testing.T) *types.K8sContextsResponse {
	t.Helper()

	resp := c.doRequest(t, "GET", "/v1/kubernetes/contexts", nil)
	if !resp.Success {
		t.Fatalf("ListContexts failed: %s", resp.Error.Message)
	}

	var result types.K8sContextsResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatalf("Failed to unmarshal contexts: %v", err)
	}

	return &result
}

// === Hostnames ===

// ListHostnames returns all hostnames added to /etc/hosts.
func (c *Client) ListHostnames(t *testing.T) []types.HostnameEntry {
	t.Helper()

	resp := c.doRequest(t, "GET", "/v1/hostnames", nil)
	if !resp.Success {
		t.Fatalf("ListHostnames failed: %s", resp.Error.Message)
	}

	var result types.HostnameListResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatalf("Failed to unmarshal hostnames: %v", err)
	}

	return result.Hostnames
}

// === Service Control ===

// ReconnectService triggers reconnection for a service.
func (c *Client) ReconnectService(t *testing.T, key string) bool {
	t.Helper()

	resp := c.doRequest(t, "POST", "/v1/service/"+url.PathEscape(key)+"/reconnect", nil)
	return resp.Success
}

// ReconnectAllErrors triggers reconnection for all services in error state.
func (c *Client) ReconnectAllErrors(t *testing.T) int {
	t.Helper()

	resp := c.doRequest(t, "POST", "/v1/services/reconnect-errors", nil)
	if !resp.Success {
		t.Fatalf("ReconnectAllErrors failed: %s", resp.Error.Message)
	}

	var result types.ReconnectResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatalf("Failed to unmarshal reconnect response: %v", err)
	}

	return result.Triggered
}

// SyncService triggers pod re-discovery for a service.
func (c *Client) SyncService(t *testing.T, key string, force bool) bool {
	t.Helper()

	path := "/v1/service/" + url.PathEscape(key) + "/sync"
	if force {
		path += "?force=true"
	}

	resp := c.doRequest(t, "POST", path, nil)
	return resp.Success
}

// === Metrics ===

// GetMetrics returns overall metrics.
func (c *Client) GetMetrics(t *testing.T) *types.MetricsSummaryResponse {
	t.Helper()

	resp := c.doRequest(t, "GET", "/v1/metrics", nil)
	if !resp.Success {
		t.Fatalf("GetMetrics failed: %s", resp.Error.Message)
	}

	var result types.MetricsSummaryResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatalf("Failed to unmarshal metrics: %v", err)
	}

	return &result
}

// === Logs ===

// GetLogs returns recent log entries.
func (c *Client) GetLogs(t *testing.T, count int, level, search string) []types.LogEntryResponse {
	t.Helper()

	params := url.Values{}
	if count > 0 {
		params.Set("count", fmt.Sprintf("%d", count))
	}
	if level != "" {
		params.Set("level", level)
	}
	if search != "" {
		params.Set("search", search)
	}

	path := "/v1/logs"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	resp := c.doRequest(t, "GET", path, nil)
	if !resp.Success {
		t.Fatalf("GetLogs failed: %s", resp.Error.Message)
	}

	var result types.LogsResponse
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatalf("Failed to unmarshal logs: %v", err)
	}

	return result.Logs
}
