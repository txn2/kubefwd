package fwdmcp

import (
	"fmt"
	"strings"
)

// MCPError represents a structured error response for MCP tools.
// Provides actionable information to help AI agents recover from errors.
type MCPError struct {
	// Code is a machine-readable error code
	Code string `json:"code"`

	// Message is a human-readable error description
	Message string `json:"message"`

	// Diagnosis explains the likely cause
	Diagnosis string `json:"diagnosis,omitempty"`

	// SuggestedActions lists recommended next steps with tool calls
	SuggestedActions []SuggestedAction `json:"suggested_actions,omitempty"`

	// RetryRecommended indicates if retrying might help
	RetryRecommended bool `json:"retry_recommended"`

	// Context provides additional error context
	Context map[string]interface{} `json:"context,omitempty"`
}

// SuggestedAction represents a recommended action to resolve an error
type SuggestedAction struct {
	// Action is the tool name to call
	Action string `json:"action"`

	// Params are the suggested parameters for the tool call
	Params map[string]interface{} `json:"params,omitempty"`

	// Hint provides human-readable guidance
	Hint string `json:"hint,omitempty"`
}

// Error implements the error interface
func (e *MCPError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Error codes for common failure scenarios
const (
	ErrCodeConnectionRefused   = "connection_refused"
	ErrCodeTimeout             = "timeout"
	ErrCodePodNotFound         = "pod_not_found"
	ErrCodeServiceNotFound     = "service_not_found"
	ErrCodeNamespaceNotFound   = "namespace_not_found"
	ErrCodePermissionDenied    = "permission_denied"
	ErrCodeAPIUnavailable      = "api_unavailable"
	ErrCodeInvalidInput        = "invalid_input"
	ErrCodeProviderUnavailable = "provider_unavailable"
	ErrCodeNetworkError        = "network_error"
	ErrCodeAlreadyExists       = "already_exists"
	ErrCodeNotForwarding       = "not_forwarding"
	ErrCodeInternal            = "internal_error"
)

// NewConnectionRefusedError creates an error for connection refused scenarios
func NewConnectionRefusedError(serviceName, podName string) *MCPError {
	return &MCPError{
		Code:      ErrCodeConnectionRefused,
		Message:   fmt.Sprintf("Cannot connect to pod %s for service %s", podName, serviceName),
		Diagnosis: "Pod may not be ready, has crashed, or is not listening on the expected port",
		SuggestedActions: []SuggestedAction{
			{
				Action: "reconnect_service",
				Params: map[string]interface{}{"key": serviceName},
				Hint:   "Retry connection to the service",
			},
			{
				Action: "sync_service",
				Params: map[string]interface{}{"key": serviceName, "force": true},
				Hint:   "Force pod discovery to find healthy pods",
			},
		},
		RetryRecommended: true,
		Context: map[string]interface{}{
			"service": serviceName,
			"pod":     podName,
		},
	}
}

// NewTimeoutError creates an error for timeout scenarios
func NewTimeoutError(operation, target string) *MCPError {
	return &MCPError{
		Code:      ErrCodeTimeout,
		Message:   fmt.Sprintf("Operation '%s' timed out for %s", operation, target),
		Diagnosis: "Network congestion, slow cluster response, or network policies blocking traffic",
		SuggestedActions: []SuggestedAction{
			{
				Action: "get_quick_status",
				Hint:   "Check overall kubefwd health",
			},
			{
				Action: "diagnose_errors",
				Hint:   "Get detailed error analysis",
			},
		},
		RetryRecommended: true,
		Context: map[string]interface{}{
			"operation": operation,
			"target":    target,
		},
	}
}

// NewPodNotFoundError creates an error for pod not found scenarios
func NewPodNotFoundError(serviceName string) *MCPError {
	return &MCPError{
		Code:      ErrCodePodNotFound,
		Message:   fmt.Sprintf("No pods found for service %s", serviceName),
		Diagnosis: "Service may have no running pods, or pods are not ready",
		SuggestedActions: []SuggestedAction{
			{
				Action: "sync_service",
				Params: map[string]interface{}{"key": serviceName, "force": true},
				Hint:   "Force pod sync to discover available pods",
			},
			{
				Action: "list_k8s_services",
				Hint:   "Check service status in the cluster",
			},
		},
		RetryRecommended: true,
		Context: map[string]interface{}{
			"service": serviceName,
		},
	}
}

// NewServiceNotFoundError creates an error for service not found scenarios
func NewServiceNotFoundError(serviceKey string) *MCPError {
	return &MCPError{
		Code:      ErrCodeServiceNotFound,
		Message:   fmt.Sprintf("Service not found: %s", serviceKey),
		Diagnosis: "Service may not be forwarding, or the key format is incorrect",
		SuggestedActions: []SuggestedAction{
			{
				Action: "list_services",
				Hint:   "List currently forwarded services",
			},
			{
				Action: "find_services",
				Params: map[string]interface{}{"query": extractServiceName(serviceKey)},
				Hint:   "Search for services by name",
			},
		},
		RetryRecommended: false,
		Context: map[string]interface{}{
			"key": serviceKey,
		},
	}
}

// NewNamespaceNotFoundError creates an error for namespace not found scenarios
func NewNamespaceNotFoundError(namespace, context string) *MCPError {
	return &MCPError{
		Code:      ErrCodeNamespaceNotFound,
		Message:   fmt.Sprintf("Namespace '%s' not found in context '%s'", namespace, context),
		Diagnosis: "Namespace does not exist or you don't have permission to access it",
		SuggestedActions: []SuggestedAction{
			{
				Action: "list_k8s_namespaces",
				Params: map[string]interface{}{"context": context},
				Hint:   "List available namespaces",
			},
			{
				Action: "list_contexts",
				Hint:   "Verify you're using the correct context",
			},
		},
		RetryRecommended: false,
		Context: map[string]interface{}{
			"namespace": namespace,
			"context":   context,
		},
	}
}

// NewProviderUnavailableError creates an error when a provider is not available
func NewProviderUnavailableError(providerName, requirement string) *MCPError {
	return &MCPError{
		Code:      ErrCodeProviderUnavailable,
		Message:   fmt.Sprintf("%s not available", providerName),
		Diagnosis: "kubefwd may not be running. Start it with: sudo -E kubefwd",
		SuggestedActions: []SuggestedAction{
			{
				Action: "get_health",
				Hint:   "Check kubefwd status",
			},
		},
		RetryRecommended: true,
		Context: map[string]interface{}{
			"provider":    providerName,
			"requirement": requirement,
		},
	}
}

// NewAPIUnavailableError creates an error when the REST API is not reachable
func NewAPIUnavailableError(apiURL string, cause error) *MCPError {
	return &MCPError{
		Code:      ErrCodeAPIUnavailable,
		Message:   fmt.Sprintf("Cannot connect to kubefwd API at %s", apiURL),
		Diagnosis: "kubefwd daemon may not be running, or the API endpoint is incorrect",
		SuggestedActions: []SuggestedAction{
			{
				Hint: "Start kubefwd in idle mode: sudo -E kubefwd",
			},
			{
				Hint: "Or with a namespace: sudo -E kubefwd svc -n <namespace>",
			},
		},
		RetryRecommended: true,
		Context: map[string]interface{}{
			"api_url": apiURL,
			"error":   cause.Error(),
		},
	}
}

// NewInvalidInputError creates an error for invalid input
func NewInvalidInputError(field, value, requirement string) *MCPError {
	return &MCPError{
		Code:      ErrCodeInvalidInput,
		Message:   fmt.Sprintf("Invalid value for '%s': %s", field, value),
		Diagnosis: requirement,
		SuggestedActions: []SuggestedAction{
			{
				Hint: fmt.Sprintf("Provide a valid value for '%s': %s", field, requirement),
			},
		},
		RetryRecommended: false,
		Context: map[string]interface{}{
			"field":       field,
			"value":       value,
			"requirement": requirement,
		},
	}
}

// NewAlreadyExistsError creates an error for duplicate resources
func NewAlreadyExistsError(resourceType, key string) *MCPError {
	actions := []SuggestedAction{
		{
			Action: "list_services",
			Hint:   "List existing forwards",
		},
	}

	if resourceType == "service" {
		actions = append(actions, SuggestedAction{
			Action: "get_connection_info",
			Params: map[string]interface{}{"service_name": extractServiceName(key)},
			Hint:   "Get connection info for existing forward",
		})
	}

	return &MCPError{
		Code:             ErrCodeAlreadyExists,
		Message:          fmt.Sprintf("%s '%s' is already being forwarded", resourceType, key),
		Diagnosis:        "The resource is already active. Use the existing forward or remove it first.",
		SuggestedActions: actions,
		RetryRecommended: false,
		Context: map[string]interface{}{
			"type": resourceType,
			"key":  key,
		},
	}
}

// NewNotForwardingError creates an error when a service is not being forwarded
func NewNotForwardingError(serviceKey string) *MCPError {
	return &MCPError{
		Code:      ErrCodeNotForwarding,
		Message:   fmt.Sprintf("Service '%s' is not currently being forwarded", serviceKey),
		Diagnosis: "The service needs to be added before this operation can be performed",
		SuggestedActions: []SuggestedAction{
			{
				Action: "list_services",
				Hint:   "List currently forwarded services",
			},
			{
				Action: "add_service",
				Hint:   "Add the service to start forwarding",
			},
		},
		RetryRecommended: false,
		Context: map[string]interface{}{
			"service": serviceKey,
		},
	}
}

// ClassifyError analyzes an error string and returns a structured MCPError
func ClassifyError(err error, context map[string]interface{}) *MCPError {
	if err == nil {
		return nil
	}

	errMsg := strings.ToLower(err.Error())

	// Extract context values with defaults
	service, _ := context["service"].(string)
	pod, _ := context["pod"].(string)
	namespace, _ := context["namespace"].(string)
	contextName, _ := context["context"].(string)

	// Classify based on error message patterns
	switch {
	case strings.Contains(errMsg, "connection refused"):
		return NewConnectionRefusedError(service, pod)

	case strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "timed out"):
		return NewTimeoutError("connection", service)

	case strings.Contains(errMsg, "not found") && strings.Contains(errMsg, "pod"):
		return NewPodNotFoundError(service)

	case strings.Contains(errMsg, "not found") && strings.Contains(errMsg, "service"):
		return NewServiceNotFoundError(service)

	case strings.Contains(errMsg, "not found") && strings.Contains(errMsg, "namespace"):
		return NewNamespaceNotFoundError(namespace, contextName)

	case strings.Contains(errMsg, "permission denied") || strings.Contains(errMsg, "forbidden"):
		return &MCPError{
			Code:      ErrCodePermissionDenied,
			Message:   "Permission denied",
			Diagnosis: "RBAC permissions may be insufficient for this operation",
			SuggestedActions: []SuggestedAction{
				{
					Action: "list_contexts",
					Hint:   "Check you're using the correct context",
				},
			},
			RetryRecommended: false,
			Context:          context,
		}

	case strings.Contains(errMsg, "already exists") || strings.Contains(errMsg, "already forwarding"):
		return NewAlreadyExistsError("service", service)

	default:
		// Generic error with context
		return &MCPError{
			Code:             ErrCodeInternal,
			Message:          err.Error(),
			Diagnosis:        "An unexpected error occurred",
			RetryRecommended: true,
			Context:          context,
		}
	}
}

// extractServiceName extracts the service name from a key like "myservice.namespace.context"
func extractServiceName(key string) string {
	parts := strings.Split(key, ".")
	if len(parts) > 0 {
		return parts[0]
	}
	return key
}
