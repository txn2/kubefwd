// Package fwdmcp provides MCP server functionality for kubefwd.
// This file contains HTTP client implementations of the interfaces
// used by the MCP server, allowing it to connect to the REST API
// instead of requiring direct memory access to kubefwd state.
package fwdmcp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/txn2/kubefwd/pkg/fwdapi/types"
	"github.com/txn2/kubefwd/pkg/fwdmetrics"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

// HTTPClient wraps http.Client with base URL
type HTTPClient struct {
	client  *http.Client
	baseURL string
}

// NewHTTPClient creates a new HTTP client for the kubefwd API
func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: baseURL,
	}
}

// Get performs a GET request and decodes JSON response
func (c *HTTPClient) Get(path string, result interface{}) error {
	resp, err := c.client.Get(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

// Post performs a POST request and decodes JSON response
func (c *HTTPClient) Post(path string, result interface{}) error {
	resp, err := c.client.Post(c.baseURL+path, "application/json", nil)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// ============================================================================
// StateReaderHTTP implements types.StateReader via REST API
// ============================================================================

type StateReaderHTTP struct {
	client *HTTPClient
}

// NewStateReaderHTTP creates a new HTTP-based StateReader
func NewStateReaderHTTP(baseURL string) *StateReaderHTTP {
	return &StateReaderHTTP{
		client: NewHTTPClient(baseURL),
	}
}

func (r *StateReaderHTTP) GetServices() []state.ServiceSnapshot {
	var resp struct {
		Success bool                      `json:"success"`
		Data    types.ServiceListResponse `json:"data"`
	}

	if err := r.client.Get("/v1/services", &resp); err != nil {
		return nil
	}

	return convertServiceResponses(resp.Data.Services)
}

func (r *StateReaderHTTP) GetService(key string) *state.ServiceSnapshot {
	var resp struct {
		Success bool                  `json:"success"`
		Data    types.ServiceResponse `json:"data"`
	}

	if err := r.client.Get("/v1/services/"+url.PathEscape(key), &resp); err != nil {
		return nil
	}

	snapshot := convertServiceResponse(resp.Data)
	return &snapshot
}

func (r *StateReaderHTTP) GetSummary() state.SummaryStats {
	var resp struct {
		Success bool                      `json:"success"`
		Data    types.ServiceListResponse `json:"data"`
	}

	if err := r.client.Get("/v1/services", &resp); err != nil {
		return state.SummaryStats{}
	}

	return state.SummaryStats{
		TotalServices:  resp.Data.Summary.TotalServices,
		ActiveServices: resp.Data.Summary.ActiveServices,
		TotalForwards:  resp.Data.Summary.TotalForwards,
		ActiveForwards: resp.Data.Summary.ActiveForwards,
		ErrorCount:     resp.Data.Summary.ErrorCount,
		TotalBytesIn:   resp.Data.Summary.TotalBytesIn,
		TotalBytesOut:  resp.Data.Summary.TotalBytesOut,
	}
}

func (r *StateReaderHTTP) GetFiltered() []state.ForwardSnapshot {
	var resp struct {
		Success bool                      `json:"success"`
		Data    types.ForwardListResponse `json:"data"`
	}

	if err := r.client.Get("/v1/forwards", &resp); err != nil {
		return nil
	}

	return convertForwardResponses(resp.Data.Forwards)
}

func (r *StateReaderHTTP) GetForward(key string) *state.ForwardSnapshot {
	var resp struct {
		Success bool                  `json:"success"`
		Data    types.ForwardResponse `json:"data"`
	}

	if err := r.client.Get("/v1/forwards/"+url.PathEscape(key), &resp); err != nil {
		return nil
	}

	snapshot := convertForwardResponse(resp.Data)
	return &snapshot
}

func (r *StateReaderHTTP) GetLogs(count int) []state.LogEntry {
	var resp struct {
		Success bool               `json:"success"`
		Data    types.LogsResponse `json:"data"`
	}

	path := fmt.Sprintf("/v1/logs?count=%d", count)
	if err := r.client.Get(path, &resp); err != nil {
		return nil
	}

	logs := make([]state.LogEntry, len(resp.Data.Logs))
	for i, l := range resp.Data.Logs {
		logs[i] = state.LogEntry{
			Timestamp: l.Timestamp,
			Level:     l.Level,
			Message:   l.Message,
		}
	}
	return logs
}

func (r *StateReaderHTTP) Count() int {
	var resp struct {
		Success bool                      `json:"success"`
		Data    types.ForwardListResponse `json:"data"`
	}

	if err := r.client.Get("/v1/forwards", &resp); err != nil {
		return 0
	}

	return len(resp.Data.Forwards)
}

func (r *StateReaderHTTP) ServiceCount() int {
	var resp struct {
		Success bool                      `json:"success"`
		Data    types.ServiceListResponse `json:"data"`
	}

	if err := r.client.Get("/v1/services", &resp); err != nil {
		return 0
	}

	return len(resp.Data.Services)
}

// ============================================================================
// MetricsProviderHTTP implements types.MetricsProvider via REST API
// ============================================================================

type MetricsProviderHTTP struct {
	client *HTTPClient
}

// NewMetricsProviderHTTP creates a new HTTP-based MetricsProvider
func NewMetricsProviderHTTP(baseURL string) *MetricsProviderHTTP {
	return &MetricsProviderHTTP{
		client: NewHTTPClient(baseURL),
	}
}

func (m *MetricsProviderHTTP) GetAllSnapshots() []fwdmetrics.ServiceSnapshot {
	var resp struct {
		Success bool                           `json:"success"`
		Data    []types.ServiceMetricsResponse `json:"data"`
	}

	if err := m.client.Get("/v1/metrics/services", &resp); err != nil {
		return nil
	}

	snapshots := make([]fwdmetrics.ServiceSnapshot, len(resp.Data))
	for i, s := range resp.Data {
		snapshots[i] = fwdmetrics.ServiceSnapshot{
			ServiceName:   s.ServiceName,
			Namespace:     s.Namespace,
			Context:       s.Context,
			TotalBytesIn:  s.TotalBytesIn,
			TotalBytesOut: s.TotalBytesOut,
			TotalRateIn:   s.RateIn,
			TotalRateOut:  s.RateOut,
		}
	}
	return snapshots
}

func (m *MetricsProviderHTTP) GetServiceSnapshot(key string) *fwdmetrics.ServiceSnapshot {
	var resp struct {
		Success bool                         `json:"success"`
		Data    types.ServiceMetricsResponse `json:"data"`
	}

	if err := m.client.Get("/v1/metrics/services/"+url.PathEscape(key), &resp); err != nil {
		return nil
	}

	return &fwdmetrics.ServiceSnapshot{
		ServiceName:   resp.Data.ServiceName,
		Namespace:     resp.Data.Namespace,
		Context:       resp.Data.Context,
		TotalBytesIn:  resp.Data.TotalBytesIn,
		TotalBytesOut: resp.Data.TotalBytesOut,
		TotalRateIn:   resp.Data.RateIn,
		TotalRateOut:  resp.Data.RateOut,
	}
}

func (m *MetricsProviderHTTP) GetTotals() (bytesIn, bytesOut uint64, rateIn, rateOut float64) {
	var resp struct {
		Success bool                         `json:"success"`
		Data    types.MetricsSummaryResponse `json:"data"`
	}

	if err := m.client.Get("/v1/metrics", &resp); err != nil {
		return 0, 0, 0, 0
	}

	return resp.Data.TotalBytesIn, resp.Data.TotalBytesOut, resp.Data.TotalRateIn, resp.Data.TotalRateOut
}

func (m *MetricsProviderHTTP) ServiceCount() int {
	var resp struct {
		Success bool                         `json:"success"`
		Data    types.MetricsSummaryResponse `json:"data"`
	}

	if err := m.client.Get("/v1/metrics", &resp); err != nil {
		return 0
	}

	return resp.Data.TotalServices
}

func (m *MetricsProviderHTTP) PortForwardCount() int {
	var resp struct {
		Success bool                         `json:"success"`
		Data    types.MetricsSummaryResponse `json:"data"`
	}

	if err := m.client.Get("/v1/metrics", &resp); err != nil {
		return 0
	}

	return resp.Data.TotalForwards
}

// ============================================================================
// ServiceControllerHTTP implements types.ServiceController via REST API
// ============================================================================

type ServiceControllerHTTP struct {
	client *HTTPClient
}

// NewServiceControllerHTTP creates a new HTTP-based ServiceController
func NewServiceControllerHTTP(baseURL string) *ServiceControllerHTTP {
	return &ServiceControllerHTTP{
		client: NewHTTPClient(baseURL),
	}
}

func (c *ServiceControllerHTTP) Reconnect(key string) error {
	return c.client.Post("/v1/services/"+url.PathEscape(key)+"/reconnect", nil)
}

func (c *ServiceControllerHTTP) ReconnectAll() int {
	var resp struct {
		Success bool                    `json:"success"`
		Data    types.ReconnectResponse `json:"data"`
	}

	if err := c.client.Post("/v1/services/reconnect", &resp); err != nil {
		return 0
	}

	return resp.Data.Triggered
}

func (c *ServiceControllerHTTP) Sync(key string, force bool) error {
	path := fmt.Sprintf("/v1/services/%s/sync?force=%v", url.PathEscape(key), force)
	return c.client.Post(path, nil)
}

// ============================================================================
// DiagnosticsProviderHTTP implements types.DiagnosticsProvider via REST API
// ============================================================================

type DiagnosticsProviderHTTP struct {
	client *HTTPClient
}

// NewDiagnosticsProviderHTTP creates a new HTTP-based DiagnosticsProvider
func NewDiagnosticsProviderHTTP(baseURL string) *DiagnosticsProviderHTTP {
	return &DiagnosticsProviderHTTP{
		client: NewHTTPClient(baseURL),
	}
}

func (d *DiagnosticsProviderHTTP) GetSummary() types.DiagnosticSummary {
	var resp struct {
		Success bool                    `json:"success"`
		Data    types.DiagnosticSummary `json:"data"`
	}

	if err := d.client.Get("/v1/diagnostics", &resp); err != nil {
		return types.DiagnosticSummary{}
	}

	return resp.Data
}

func (d *DiagnosticsProviderHTTP) GetServiceDiagnostic(key string) (*types.ServiceDiagnostic, error) {
	var resp struct {
		Success bool                    `json:"success"`
		Data    types.ServiceDiagnostic `json:"data"`
		Error   *types.ErrorInfo        `json:"error"`
	}

	if err := d.client.Get("/v1/diagnostics/services/"+url.PathEscape(key), &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return &resp.Data, nil
}

func (d *DiagnosticsProviderHTTP) GetForwardDiagnostic(key string) (*types.ForwardDiagnostic, error) {
	var resp struct {
		Success bool                    `json:"success"`
		Data    types.ForwardDiagnostic `json:"data"`
		Error   *types.ErrorInfo        `json:"error"`
	}

	if err := d.client.Get("/v1/diagnostics/forwards/"+url.PathEscape(key), &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return &resp.Data, nil
}

func (d *DiagnosticsProviderHTTP) GetNetworkStatus() types.NetworkStatus {
	var resp struct {
		Success bool                `json:"success"`
		Data    types.NetworkStatus `json:"data"`
	}

	if err := d.client.Get("/v1/diagnostics/network", &resp); err != nil {
		return types.NetworkStatus{}
	}

	return resp.Data
}

func (d *DiagnosticsProviderHTTP) GetErrors(count int) []types.ErrorDetail {
	var resp struct {
		Success bool                `json:"success"`
		Data    []types.ErrorDetail `json:"data"`
	}

	path := fmt.Sprintf("/v1/diagnostics/errors?count=%d", count)
	if err := d.client.Get(path, &resp); err != nil {
		return nil
	}

	return resp.Data
}

// ============================================================================
// ManagerInfoHTTP implements types.ManagerInfo via REST API
// ============================================================================

type ManagerInfoHTTP struct {
	client    *HTTPClient
	startTime time.Time
}

// NewManagerInfoHTTP creates a new HTTP-based ManagerInfo
func NewManagerInfoHTTP(baseURL string) *ManagerInfoHTTP {
	m := &ManagerInfoHTTP{
		client:    NewHTTPClient(baseURL),
		startTime: time.Now(),
	}
	// Fetch actual start time
	m.refresh()
	return m
}

type infoCache struct {
	Version    string
	StartTime  time.Time
	Namespaces []string
	Contexts   []string
	TUIEnabled bool
}

func (m *ManagerInfoHTTP) refresh() *infoCache {
	var resp struct {
		Success bool               `json:"success"`
		Data    types.InfoResponse `json:"data"`
	}

	if err := m.client.Get("/info", &resp); err != nil {
		return nil
	}

	m.startTime = resp.Data.StartTime
	return &infoCache{
		Version:    resp.Data.Version,
		StartTime:  resp.Data.StartTime,
		Namespaces: resp.Data.Namespaces,
		Contexts:   resp.Data.Contexts,
		TUIEnabled: resp.Data.TUIEnabled,
	}
}

func (m *ManagerInfoHTTP) Version() string {
	if info := m.refresh(); info != nil {
		return info.Version
	}
	return "unknown"
}

func (m *ManagerInfoHTTP) Uptime() time.Duration {
	return time.Since(m.startTime)
}

func (m *ManagerInfoHTTP) StartTime() time.Time {
	return m.startTime
}

func (m *ManagerInfoHTTP) Namespaces() []string {
	if info := m.refresh(); info != nil {
		return info.Namespaces
	}
	return nil
}

func (m *ManagerInfoHTTP) Contexts() []string {
	if info := m.refresh(); info != nil {
		return info.Contexts
	}
	return nil
}

func (m *ManagerInfoHTTP) TUIEnabled() bool {
	if info := m.refresh(); info != nil {
		return info.TUIEnabled
	}
	return false
}

// ============================================================================
// Helper conversion functions
// ============================================================================

func convertServiceResponses(services []types.ServiceResponse) []state.ServiceSnapshot {
	result := make([]state.ServiceSnapshot, len(services))
	for i, s := range services {
		result[i] = convertServiceResponse(s)
	}
	return result
}

func convertServiceResponse(s types.ServiceResponse) state.ServiceSnapshot {
	return state.ServiceSnapshot{
		Key:           s.Key,
		ServiceName:   s.ServiceName,
		Namespace:     s.Namespace,
		Context:       s.Context,
		Headless:      s.Headless,
		ActiveCount:   s.ActiveCount,
		ErrorCount:    s.ErrorCount,
		TotalBytesIn:  s.TotalBytesIn,
		TotalBytesOut: s.TotalBytesOut,
		PortForwards:  convertForwardResponses(s.Forwards),
	}
}

func convertForwardResponses(forwards []types.ForwardResponse) []state.ForwardSnapshot {
	result := make([]state.ForwardSnapshot, len(forwards))
	for i, f := range forwards {
		result[i] = convertForwardResponse(f)
	}
	return result
}

func convertForwardResponse(f types.ForwardResponse) state.ForwardSnapshot {
	return state.ForwardSnapshot{
		Key:           f.Key,
		ServiceKey:    f.ServiceKey,
		ServiceName:   f.ServiceName,
		Namespace:     f.Namespace,
		Context:       f.Context,
		Headless:      f.Headless,
		PodName:       f.PodName,
		ContainerName: f.ContainerName,
		LocalIP:       f.LocalIP,
		LocalPort:     f.LocalPort,
		PodPort:       f.PodPort,
		Hostnames:     f.Hostnames,
		Status:        parseForwardStatus(f.Status),
		Error:         f.Error,
		StartedAt:     f.StartedAt,
		LastActive:    f.LastActive,
		BytesIn:       f.BytesIn,
		BytesOut:      f.BytesOut,
		RateIn:        f.RateIn,
		RateOut:       f.RateOut,
	}
}

func parseForwardStatus(s string) state.ForwardStatus {
	switch s {
	case "active":
		return state.StatusActive
	case "connecting":
		return state.StatusConnecting
	case "error":
		return state.StatusError
	case "stopped", "stopping":
		return state.StatusStopping
	default:
		return state.StatusPending
	}
}

// parsePort converts a string port to int, returns 0 on error
func parsePort(s string) int {
	p, _ := strconv.Atoi(s)
	return p
}
