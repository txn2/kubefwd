// Package fwdmcp provides MCP server functionality for kubefwd.
// This file contains HTTP client implementations of the interfaces
// used by the MCP server, allowing it to connect to the REST API
// instead of requiring direct memory access to kubefwd state.
package fwdmcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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
	defer func() { _ = resp.Body.Close() }()

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
	defer func() { _ = resp.Body.Close() }()

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

// PostJSON performs a POST request with JSON body and decodes JSON response
func (c *HTTPClient) PostJSON(path string, body interface{}, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	resp, err := c.client.Post(c.baseURL+path, "application/json", bodyReader)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// Delete performs a DELETE request and decodes JSON response
func (c *HTTPClient) Delete(path string, result interface{}) error {
	req, err := http.NewRequest(http.MethodDelete, c.baseURL+path, http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	if result != nil && resp.StatusCode != http.StatusNoContent {
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

// ============================================================================
// NamespaceControllerHTTP implements types.NamespaceController via REST API
// ============================================================================

type NamespaceControllerHTTP struct {
	client *HTTPClient
}

// NewNamespaceControllerHTTP creates a new HTTP-based NamespaceController
func NewNamespaceControllerHTTP(baseURL string) *NamespaceControllerHTTP {
	return &NamespaceControllerHTTP{
		client: NewHTTPClient(baseURL),
	}
}

func (n *NamespaceControllerHTTP) AddNamespace(ctx, namespace string, opts types.AddNamespaceOpts) (*types.NamespaceInfoResponse, error) {
	req := types.AddNamespaceRequest{
		Namespace: namespace,
		Context:   ctx,
		Selector:  opts.LabelSelector,
	}

	var resp struct {
		Success bool                       `json:"success"`
		Data    types.AddNamespaceResponse `json:"data"`
		Error   *types.ErrorInfo           `json:"error"`
	}

	if err := n.client.PostJSON("/v1/namespaces", req, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return &types.NamespaceInfoResponse{
		Key:          resp.Data.Key,
		Namespace:    resp.Data.Namespace,
		Context:      resp.Data.Context,
		ServiceCount: len(resp.Data.Services),
	}, nil
}

func (n *NamespaceControllerHTTP) RemoveNamespace(ctx, namespace string) error {
	key := namespace
	if ctx != "" {
		key = namespace + "." + ctx
	}

	var resp struct {
		Success bool             `json:"success"`
		Error   *types.ErrorInfo `json:"error"`
	}

	if err := n.client.Delete("/v1/namespaces/"+url.PathEscape(key), &resp); err != nil {
		return err
	}

	if resp.Error != nil {
		return fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return nil
}

func (n *NamespaceControllerHTTP) ListNamespaces() []types.NamespaceInfoResponse {
	var resp struct {
		Success bool                        `json:"success"`
		Data    types.NamespaceListResponse `json:"data"`
	}

	if err := n.client.Get("/v1/namespaces", &resp); err != nil {
		return nil
	}

	return resp.Data.Namespaces
}

func (n *NamespaceControllerHTTP) GetNamespace(ctx, namespace string) (*types.NamespaceInfoResponse, error) {
	key := namespace
	if ctx != "" {
		key = namespace + "." + ctx
	}

	var resp struct {
		Success bool                        `json:"success"`
		Data    types.NamespaceInfoResponse `json:"data"`
		Error   *types.ErrorInfo            `json:"error"`
	}

	if err := n.client.Get("/v1/namespaces/"+url.PathEscape(key), &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return &resp.Data, nil
}

// ============================================================================
// ServiceCRUDHTTP implements types.ServiceCRUD via REST API
// ============================================================================

type ServiceCRUDHTTP struct {
	*ServiceControllerHTTP
}

// NewServiceCRUDHTTP creates a new HTTP-based ServiceCRUD
func NewServiceCRUDHTTP(baseURL string) *ServiceCRUDHTTP {
	return &ServiceCRUDHTTP{
		ServiceControllerHTTP: NewServiceControllerHTTP(baseURL),
	}
}

func (s *ServiceCRUDHTTP) AddService(req types.AddServiceRequest) (*types.AddServiceResponse, error) {
	var resp struct {
		Success bool                     `json:"success"`
		Data    types.AddServiceResponse `json:"data"`
		Error   *types.ErrorInfo         `json:"error"`
	}

	if err := s.client.PostJSON("/v1/services", req, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return &resp.Data, nil
}

func (s *ServiceCRUDHTTP) RemoveService(key string) error {
	var resp struct {
		Success bool             `json:"success"`
		Error   *types.ErrorInfo `json:"error"`
	}

	if err := s.client.Delete("/v1/services/"+url.PathEscape(key), &resp); err != nil {
		return err
	}

	if resp.Error != nil {
		return fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return nil
}

// ============================================================================
// KubernetesDiscoveryHTTP implements types.KubernetesDiscovery via REST API
// ============================================================================

type KubernetesDiscoveryHTTP struct {
	client *HTTPClient
}

// NewKubernetesDiscoveryHTTP creates a new HTTP-based KubernetesDiscovery
func NewKubernetesDiscoveryHTTP(baseURL string) *KubernetesDiscoveryHTTP {
	return &KubernetesDiscoveryHTTP{
		client: NewHTTPClient(baseURL),
	}
}

func (k *KubernetesDiscoveryHTTP) ListNamespaces(ctx string) ([]types.K8sNamespace, error) {
	path := "/v1/kubernetes/namespaces"
	if ctx != "" {
		path += "?context=" + url.QueryEscape(ctx)
	}

	var resp struct {
		Success bool                        `json:"success"`
		Data    types.K8sNamespacesResponse `json:"data"`
		Error   *types.ErrorInfo            `json:"error"`
	}

	if err := k.client.Get(path, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Data.Namespaces, nil
}

func (k *KubernetesDiscoveryHTTP) ListServices(ctx, namespace string) ([]types.K8sService, error) {
	path := "/v1/kubernetes/services"
	params := url.Values{}
	if ctx != "" {
		params.Set("context", ctx)
	}
	if namespace != "" {
		params.Set("namespace", namespace)
	}
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var resp struct {
		Success bool                      `json:"success"`
		Data    types.K8sServicesResponse `json:"data"`
		Error   *types.ErrorInfo          `json:"error"`
	}

	if err := k.client.Get(path, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Data.Services, nil
}

func (k *KubernetesDiscoveryHTTP) ListContexts() (*types.K8sContextsResponse, error) {
	var resp struct {
		Success bool                      `json:"success"`
		Data    types.K8sContextsResponse `json:"data"`
		Error   *types.ErrorInfo          `json:"error"`
	}

	if err := k.client.Get("/v1/kubernetes/contexts", &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return &resp.Data, nil
}

func (k *KubernetesDiscoveryHTTP) GetService(ctx, namespace, name string) (*types.K8sService, error) {
	path := fmt.Sprintf("/v1/kubernetes/services/%s/%s", url.PathEscape(namespace), url.PathEscape(name))
	if ctx != "" {
		path += "?context=" + url.QueryEscape(ctx)
	}

	var resp struct {
		Success bool             `json:"success"`
		Data    types.K8sService `json:"data"`
		Error   *types.ErrorInfo `json:"error"`
	}

	if err := k.client.Get(path, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return &resp.Data, nil
}

func (k *KubernetesDiscoveryHTTP) GetPodLogs(ctx, namespace, podName string, opts types.PodLogsOptions) (*types.PodLogsResponse, error) {
	params := url.Values{}
	if ctx != "" {
		params.Set("context", ctx)
	}
	if opts.Container != "" {
		params.Set("container", opts.Container)
	}
	if opts.TailLines > 0 {
		params.Set("tail_lines", strconv.Itoa(opts.TailLines))
	}
	if opts.SinceTime != "" {
		params.Set("since_time", opts.SinceTime)
	}
	if opts.Previous {
		params.Set("previous", "true")
	}
	if opts.Timestamps {
		params.Set("timestamps", "true")
	}

	path := fmt.Sprintf("/v1/kubernetes/pods/%s/%s/logs", url.PathEscape(namespace), url.PathEscape(podName))
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var resp struct {
		Success bool                  `json:"success"`
		Data    types.PodLogsResponse `json:"data"`
		Error   *types.ErrorInfo      `json:"error"`
	}

	if err := k.client.Get(path, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return &resp.Data, nil
}

func (k *KubernetesDiscoveryHTTP) ListPods(ctx, namespace string, opts types.ListPodsOptions) ([]types.K8sPod, error) {
	params := url.Values{}
	if ctx != "" {
		params.Set("context", ctx)
	}
	if opts.LabelSelector != "" {
		params.Set("label_selector", opts.LabelSelector)
	}
	if opts.ServiceName != "" {
		params.Set("service_name", opts.ServiceName)
	}

	path := fmt.Sprintf("/v1/kubernetes/pods/%s", url.PathEscape(namespace))
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var resp struct {
		Success bool             `json:"success"`
		Data    []types.K8sPod   `json:"data"`
		Error   *types.ErrorInfo `json:"error"`
	}

	if err := k.client.Get(path, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Data, nil
}

func (k *KubernetesDiscoveryHTTP) GetPod(ctx, namespace, podName string) (*types.K8sPodDetail, error) {
	params := url.Values{}
	if ctx != "" {
		params.Set("context", ctx)
	}

	path := fmt.Sprintf("/v1/kubernetes/pods/%s/%s", url.PathEscape(namespace), url.PathEscape(podName))
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var resp struct {
		Success bool               `json:"success"`
		Data    types.K8sPodDetail `json:"data"`
		Error   *types.ErrorInfo   `json:"error"`
	}

	if err := k.client.Get(path, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return &resp.Data, nil
}

func (k *KubernetesDiscoveryHTTP) GetEvents(ctx, namespace string, opts types.GetEventsOptions) ([]types.K8sEvent, error) {
	params := url.Values{}
	if ctx != "" {
		params.Set("context", ctx)
	}
	if opts.ResourceKind != "" {
		params.Set("resource_kind", opts.ResourceKind)
	}
	if opts.ResourceName != "" {
		params.Set("resource_name", opts.ResourceName)
	}
	if opts.Limit > 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}

	path := fmt.Sprintf("/v1/kubernetes/events/%s", url.PathEscape(namespace))
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var resp struct {
		Success bool             `json:"success"`
		Data    []types.K8sEvent `json:"data"`
		Error   *types.ErrorInfo `json:"error"`
	}

	if err := k.client.Get(path, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Data, nil
}

func (k *KubernetesDiscoveryHTTP) GetEndpoints(ctx, namespace, serviceName string) (*types.K8sEndpoints, error) {
	params := url.Values{}
	if ctx != "" {
		params.Set("context", ctx)
	}

	path := fmt.Sprintf("/v1/kubernetes/endpoints/%s/%s", url.PathEscape(namespace), url.PathEscape(serviceName))
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var resp struct {
		Success bool               `json:"success"`
		Data    types.K8sEndpoints `json:"data"`
		Error   *types.ErrorInfo   `json:"error"`
	}

	if err := k.client.Get(path, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return &resp.Data, nil
}

// ============================================================================
// ConnectionInfoProviderHTTP implements types.ConnectionInfoProvider via REST API
// ============================================================================

type ConnectionInfoProviderHTTP struct {
	client *HTTPClient
}

// NewConnectionInfoProviderHTTP creates a new HTTP-based ConnectionInfoProvider
func NewConnectionInfoProviderHTTP(baseURL string) *ConnectionInfoProviderHTTP {
	return &ConnectionInfoProviderHTTP{
		client: NewHTTPClient(baseURL),
	}
}

func (c *ConnectionInfoProviderHTTP) GetConnectionInfo(key string) (*types.ConnectionInfoResponse, error) {
	// The connection info is derived from the service details
	var resp struct {
		Success bool                  `json:"success"`
		Data    types.ServiceResponse `json:"data"`
		Error   *types.ErrorInfo      `json:"error"`
	}

	if err := c.client.Get("/v1/services/"+url.PathEscape(key), &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	svc := resp.Data

	// Build connection info from service data
	info := &types.ConnectionInfoResponse{
		Service:   svc.ServiceName,
		Namespace: svc.Namespace,
		Context:   svc.Context,
		Status:    svc.Status,
	}

	// Get hostnames and ports from forwards
	hostnamesSet := make(map[string]bool)
	var ports []types.PortInfo
	for _, fwd := range svc.Forwards {
		info.LocalIP = fwd.LocalIP
		for _, h := range fwd.Hostnames {
			hostnamesSet[h] = true
		}
		localPort := parsePort(fwd.LocalPort)
		remotePort := parsePort(fwd.PodPort)
		ports = append(ports, types.PortInfo{
			LocalPort:  localPort,
			RemotePort: remotePort,
		})
	}

	for h := range hostnamesSet {
		info.Hostnames = append(info.Hostnames, h)
	}
	info.Ports = ports

	// Generate env vars
	info.EnvVars = generateEnvVars(svc.ServiceName, info.LocalIP, ports)

	return info, nil
}

func (c *ConnectionInfoProviderHTTP) ListHostnames() (*types.HostnameListResponse, error) {
	var resp struct {
		Success bool                      `json:"success"`
		Data    types.ServiceListResponse `json:"data"`
		Error   *types.ErrorInfo          `json:"error"`
	}

	if err := c.client.Get("/v1/services", &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	// Collect all hostnames from all services
	var entries []types.HostnameEntry
	for _, svc := range resp.Data.Services {
		for _, fwd := range svc.Forwards {
			for _, hostname := range fwd.Hostnames {
				entries = append(entries, types.HostnameEntry{
					Hostname:  hostname,
					IP:        fwd.LocalIP,
					Service:   svc.ServiceName,
					Namespace: svc.Namespace,
					Context:   svc.Context,
				})
			}
		}
	}

	return &types.HostnameListResponse{
		Hostnames: entries,
		Total:     len(entries),
	}, nil
}

func (c *ConnectionInfoProviderHTTP) FindServices(query string, port int, namespace string) ([]types.ConnectionInfoResponse, error) {
	var resp struct {
		Success bool                      `json:"success"`
		Data    types.ServiceListResponse `json:"data"`
		Error   *types.ErrorInfo          `json:"error"`
	}

	if err := c.client.Get("/v1/services", &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	var results []types.ConnectionInfoResponse
	for _, svc := range resp.Data.Services {
		// Filter by namespace
		if namespace != "" && svc.Namespace != namespace {
			continue
		}

		// Filter by query (match service name)
		if query != "" && !strings.Contains(strings.ToLower(svc.ServiceName), strings.ToLower(query)) {
			continue
		}

		// Filter by port
		matchesPort := port == 0
		var ports []types.PortInfo
		hostnamesSet := make(map[string]bool)
		var localIP string

		for _, fwd := range svc.Forwards {
			localIP = fwd.LocalIP
			for _, h := range fwd.Hostnames {
				hostnamesSet[h] = true
			}
			localPort := parsePort(fwd.LocalPort)
			remotePort := parsePort(fwd.PodPort)
			if port == 0 || localPort == port || remotePort == port {
				matchesPort = true
				ports = append(ports, types.PortInfo{
					LocalPort:  localPort,
					RemotePort: remotePort,
				})
			}
		}

		if !matchesPort {
			continue
		}

		var hostnames []string
		for h := range hostnamesSet {
			hostnames = append(hostnames, h)
		}

		results = append(results, types.ConnectionInfoResponse{
			Service:   svc.ServiceName,
			Namespace: svc.Namespace,
			Context:   svc.Context,
			LocalIP:   localIP,
			Hostnames: hostnames,
			Ports:     ports,
			EnvVars:   generateEnvVars(svc.ServiceName, localIP, ports),
			Status:    svc.Status,
		})
	}

	return results, nil
}

// generateEnvVars creates environment variable suggestions for a service
func generateEnvVars(serviceName, localIP string, ports []types.PortInfo) map[string]string {
	envVars := make(map[string]string)
	upperName := strings.ToUpper(strings.ReplaceAll(serviceName, "-", "_"))

	envVars[upperName+"_HOST"] = serviceName
	if localIP != "" {
		envVars[upperName+"_IP"] = localIP
	}

	if len(ports) > 0 {
		envVars[upperName+"_PORT"] = strconv.Itoa(ports[0].LocalPort)

		// Generate connection URL based on common port patterns
		port := ports[0].LocalPort
		host := serviceName
		switch port {
		case 5432:
			envVars["DATABASE_URL"] = fmt.Sprintf("postgresql://%s:%d", host, port)
		case 3306:
			envVars["DATABASE_URL"] = fmt.Sprintf("mysql://%s:%d", host, port)
		case 27017:
			envVars["MONGODB_URI"] = fmt.Sprintf("mongodb://%s:%d", host, port)
		case 6379:
			envVars["REDIS_URL"] = fmt.Sprintf("redis://%s:%d", host, port)
		case 9092:
			envVars["KAFKA_BROKERS"] = fmt.Sprintf("%s:%d", host, port)
		case 80, 8080, 443, 8443:
			protocol := "http"
			if port == 443 || port == 8443 {
				protocol = "https"
			}
			envVars[upperName+"_URL"] = fmt.Sprintf("%s://%s:%d", protocol, host, port)
		}
	}

	return envVars
}

// ============================================================================
// AnalysisProviderHTTP provides AI-optimized analysis via REST API
// ============================================================================

// QuickStatusResponse mirrors the status endpoint response
type QuickStatusResponse struct {
	Status     string `json:"status"`     // "ok", "issues", "error"
	Message    string `json:"message"`    // Human-readable summary
	ErrorCount int    `json:"errorCount"` // Number of current errors
	Uptime     string `json:"uptime,omitempty"`
}

// AnalysisIssue represents a detected problem
type AnalysisIssue struct {
	Severity   string `json:"severity"`  // "critical", "high", "medium", "low"
	Component  string `json:"component"` // "service", "forward", "network"
	ServiceKey string `json:"serviceKey,omitempty"`
	PodName    string `json:"podName,omitempty"`
	Message    string `json:"message"`
	ErrorType  string `json:"errorType,omitempty"`
}

// AnalysisRecommendation provides actionable advice
type AnalysisRecommendation struct {
	Priority string `json:"priority"` // "high", "medium", "low"
	Category string `json:"category"` // "performance", "reliability", "configuration"
	Message  string `json:"message"`
}

// AnalysisActionSuggestion provides API actions to fix issues
type AnalysisActionSuggestion struct {
	Action   string `json:"action"` // "reconnect", "sync", "reconnect_all"
	Target   string `json:"target"` // service key or "all"
	Reason   string `json:"reason"`
	Endpoint string `json:"endpoint"` // POST /v1/services/:key/reconnect
	Method   string `json:"method"`   // POST
}

// AnalysisStats provides statistics
type AnalysisStats struct {
	TotalServices   int    `json:"totalServices"`
	ActiveServices  int    `json:"activeServices"`
	ErroredServices int    `json:"erroredServices"`
	TotalForwards   int    `json:"totalForwards"`
	ActiveForwards  int    `json:"activeForwards"`
	TotalBytesIn    uint64 `json:"totalBytesIn"`
	TotalBytesOut   uint64 `json:"totalBytesOut"`
	Uptime          string `json:"uptime,omitempty"`
}

// FullAnalysisResponse provides complete analysis for AI consumption
type FullAnalysisResponse struct {
	Status           string                     `json:"status"`
	Summary          string                     `json:"summary"`
	Issues           []AnalysisIssue            `json:"issues,omitempty"`
	Recommendations  []AnalysisRecommendation   `json:"recommendations,omitempty"`
	SuggestedActions []AnalysisActionSuggestion `json:"suggestedActions,omitempty"`
	Stats            AnalysisStats              `json:"stats"`
}

// AnalysisProviderHTTP implements analysis endpoints via REST API
type AnalysisProviderHTTP struct {
	client *HTTPClient
}

// NewAnalysisProviderHTTP creates a new HTTP-based AnalysisProvider
func NewAnalysisProviderHTTP(baseURL string) *AnalysisProviderHTTP {
	return &AnalysisProviderHTTP{
		client: NewHTTPClient(baseURL),
	}
}

// GetQuickStatus returns a quick status check
func (a *AnalysisProviderHTTP) GetQuickStatus() (*QuickStatusResponse, error) {
	var resp struct {
		Success bool                `json:"success"`
		Data    QuickStatusResponse `json:"data"`
		Error   *types.ErrorInfo    `json:"error"`
	}

	if err := a.client.Get("/v1/status", &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return &resp.Data, nil
}

// GetAnalysis returns full AI-optimized analysis
func (a *AnalysisProviderHTTP) GetAnalysis() (*FullAnalysisResponse, error) {
	var resp struct {
		Success bool                 `json:"success"`
		Data    FullAnalysisResponse `json:"data"`
		Error   *types.ErrorInfo     `json:"error"`
	}

	if err := a.client.Get("/v1/analyze", &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return &resp.Data, nil
}

// ============================================================================
// HTTPTrafficProviderHTTP provides HTTP traffic inspection via REST API
// ============================================================================

// HTTPTrafficProviderHTTP implements HTTP traffic endpoints via REST API
type HTTPTrafficProviderHTTP struct {
	client *HTTPClient
}

// NewHTTPTrafficProviderHTTP creates a new HTTP-based HTTPTrafficProvider
func NewHTTPTrafficProviderHTTP(baseURL string) *HTTPTrafficProviderHTTP {
	return &HTTPTrafficProviderHTTP{
		client: NewHTTPClient(baseURL),
	}
}

// GetForwardHTTP returns HTTP logs for a specific forward
func (h *HTTPTrafficProviderHTTP) GetForwardHTTP(key string, count int) (*types.HTTPTrafficResponse, error) {
	if count <= 0 {
		count = 50
	}
	if count > 500 {
		count = 500
	}

	path := fmt.Sprintf("/v1/forwards/%s/http?count=%d", url.PathEscape(key), count)

	var resp struct {
		Success bool                      `json:"success"`
		Data    types.HTTPTrafficResponse `json:"data"`
		Error   *types.ErrorInfo          `json:"error"`
	}

	if err := h.client.Get(path, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return &resp.Data, nil
}

// GetServiceHTTP returns HTTP logs for all forwards of a service
func (h *HTTPTrafficProviderHTTP) GetServiceHTTP(key string, count int) (*types.ServiceHTTPTrafficResponse, error) {
	if count <= 0 {
		count = 50
	}
	if count > 500 {
		count = 500
	}

	path := fmt.Sprintf("/v1/services/%s/http?count=%d", url.PathEscape(key), count)

	var resp struct {
		Success bool                             `json:"success"`
		Data    types.ServiceHTTPTrafficResponse `json:"data"`
		Error   *types.ErrorInfo                 `json:"error"`
	}

	if err := h.client.Get(path, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return &resp.Data, nil
}

// ============================================================================
// HistoryProviderHTTP provides history access via REST API
// ============================================================================

// HistoryEvent represents a historical event
type HistoryEvent struct {
	ID         int64                  `json:"id"`
	Type       string                 `json:"type"`
	Timestamp  time.Time              `json:"timestamp"`
	ServiceKey string                 `json:"serviceKey,omitempty"`
	ForwardKey string                 `json:"forwardKey,omitempty"`
	PodName    string                 `json:"podName,omitempty"`
	Message    string                 `json:"message"`
	Data       map[string]interface{} `json:"data,omitempty"`
}

// HistoryError represents a historical error
type HistoryError struct {
	ID         int64     `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	ServiceKey string    `json:"serviceKey"`
	ForwardKey string    `json:"forwardKey,omitempty"`
	PodName    string    `json:"podName,omitempty"`
	ErrorType  string    `json:"errorType"`
	Message    string    `json:"message"`
	Resolved   bool      `json:"resolved"`
	ResolvedAt time.Time `json:"resolvedAt,omitempty"`
}

// HistoryReconnect represents a reconnection attempt
type HistoryReconnect struct {
	ID           int64  `json:"id"`
	Timestamp    string `json:"timestamp"` // Duration string from JSON
	ServiceKey   string `json:"serviceKey"`
	Trigger      string `json:"trigger"` // "auto", "manual", "sync"
	AttemptCount int    `json:"attemptCount"`
	Success      bool   `json:"success"`
	Duration     string `json:"duration"` // Duration string from JSON
	Error        string `json:"error,omitempty"`
}

// HistoryStats provides statistics about stored history
type HistoryStats struct {
	TotalEvents     int `json:"totalEvents"`
	TotalErrors     int `json:"totalErrors"`
	TotalReconnects int `json:"totalReconnects"`
	MaxEvents       int `json:"maxEvents"`
	MaxErrors       int `json:"maxErrors"`
	MaxReconnects   int `json:"maxReconnects"`
}

// HistoryProviderHTTP implements history endpoints via REST API
type HistoryProviderHTTP struct {
	client *HTTPClient
}

// NewHistoryProviderHTTP creates a new HTTP-based HistoryProvider
func NewHistoryProviderHTTP(baseURL string) *HistoryProviderHTTP {
	return &HistoryProviderHTTP{
		client: NewHTTPClient(baseURL),
	}
}

// GetEvents returns historical events
func (h *HistoryProviderHTTP) GetEvents(count int, eventType string) ([]HistoryEvent, error) {
	if count <= 0 {
		count = 100
	}
	if count > 1000 {
		count = 1000
	}

	path := fmt.Sprintf("/v1/history/events?count=%d", count)
	if eventType != "" {
		path += "&type=" + url.QueryEscape(eventType)
	}

	var resp struct {
		Success bool           `json:"success"`
		Data    []HistoryEvent `json:"data"`
		Error   *types.ErrorInfo
	}

	if err := h.client.Get(path, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Data, nil
}

// GetErrors returns historical errors
func (h *HistoryProviderHTTP) GetErrors(count int) ([]HistoryError, error) {
	if count <= 0 {
		count = 50
	}
	if count > 500 {
		count = 500
	}

	path := fmt.Sprintf("/v1/history/errors?count=%d", count)

	var resp struct {
		Success bool           `json:"success"`
		Data    []HistoryError `json:"data"`
		Error   *types.ErrorInfo
	}

	if err := h.client.Get(path, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Data, nil
}

// GetReconnects returns reconnection history
func (h *HistoryProviderHTTP) GetReconnects(count int, serviceKey string) ([]HistoryReconnect, error) {
	if count <= 0 {
		count = 50
	}
	if count > 200 {
		count = 200
	}

	var path string
	if serviceKey != "" {
		path = fmt.Sprintf("/v1/services/%s/history/reconnections?count=%d",
			url.PathEscape(serviceKey), count)
	} else {
		path = fmt.Sprintf("/v1/history/reconnections?count=%d", count)
	}

	var resp struct {
		Success bool               `json:"success"`
		Data    []HistoryReconnect `json:"data"`
		Error   *types.ErrorInfo
	}

	if err := h.client.Get(path, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Data, nil
}

// GetStats returns history storage statistics
func (h *HistoryProviderHTTP) GetStats() (*HistoryStats, error) {
	var resp struct {
		Success bool             `json:"success"`
		Data    HistoryStats     `json:"data"`
		Error   *types.ErrorInfo `json:"error"`
	}

	if err := h.client.Get("/v1/history/stats", &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return &resp.Data, nil
}
