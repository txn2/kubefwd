package fwdmcp

import (
	"errors"
	"testing"
)

func TestMCPError_Error(t *testing.T) {
	err := &MCPError{
		Code:    ErrCodeServiceNotFound,
		Message: "service not found: myapp.default.cluster",
	}

	expected := "service_not_found: service not found: myapp.default.cluster"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestNewConnectionRefusedError(t *testing.T) {
	err := NewConnectionRefusedError("myapp.default", "myapp-pod-123")

	if err.Code != ErrCodeConnectionRefused {
		t.Errorf("expected code %s, got %s", ErrCodeConnectionRefused, err.Code)
	}

	if !err.RetryRecommended {
		t.Error("expected retry to be recommended")
	}

	if len(err.SuggestedActions) != 2 {
		t.Errorf("expected 2 suggested actions, got %d", len(err.SuggestedActions))
	}

	// Check first action is reconnect
	if err.SuggestedActions[0].Action != "reconnect_service" {
		t.Errorf("expected first action to be reconnect_service, got %s", err.SuggestedActions[0].Action)
	}

	// Check context
	if err.Context["service"] != "myapp.default" {
		t.Errorf("expected service context, got %v", err.Context["service"])
	}
}

func TestNewTimeoutError(t *testing.T) {
	err := NewTimeoutError("connection", "myapp.default")

	if err.Code != ErrCodeTimeout {
		t.Errorf("expected code %s, got %s", ErrCodeTimeout, err.Code)
	}

	if !err.RetryRecommended {
		t.Error("expected retry to be recommended")
	}
}

func TestNewPodNotFoundError(t *testing.T) {
	err := NewPodNotFoundError("myapp.default")

	if err.Code != ErrCodePodNotFound {
		t.Errorf("expected code %s, got %s", ErrCodePodNotFound, err.Code)
	}

	// Should have sync_service as first suggestion
	if len(err.SuggestedActions) < 1 {
		t.Fatal("expected at least 1 suggested action")
	}
	if err.SuggestedActions[0].Action != "sync_service" {
		t.Errorf("expected sync_service action, got %s", err.SuggestedActions[0].Action)
	}
}

func TestNewServiceNotFoundError(t *testing.T) {
	err := NewServiceNotFoundError("myapp.default.cluster")

	if err.Code != ErrCodeServiceNotFound {
		t.Errorf("expected code %s, got %s", ErrCodeServiceNotFound, err.Code)
	}

	if err.RetryRecommended {
		t.Error("expected retry NOT to be recommended for service not found")
	}

	// Check list_services is suggested
	if len(err.SuggestedActions) < 1 {
		t.Fatal("expected at least 1 suggested action")
	}
	if err.SuggestedActions[0].Action != "list_services" {
		t.Errorf("expected list_services action, got %s", err.SuggestedActions[0].Action)
	}
}

func TestNewNamespaceNotFoundError(t *testing.T) {
	err := NewNamespaceNotFoundError("staging", "prod-cluster")

	if err.Code != ErrCodeNamespaceNotFound {
		t.Errorf("expected code %s, got %s", ErrCodeNamespaceNotFound, err.Code)
	}

	if err.RetryRecommended {
		t.Error("expected retry NOT to be recommended")
	}

	if err.Context["namespace"] != "staging" {
		t.Errorf("expected namespace context, got %v", err.Context["namespace"])
	}
	if err.Context["context"] != "prod-cluster" {
		t.Errorf("expected context context, got %v", err.Context["context"])
	}
}

func TestNewProviderUnavailableError(t *testing.T) {
	err := NewProviderUnavailableError("State reader", "make sure kubefwd is running")

	if err.Code != ErrCodeProviderUnavailable {
		t.Errorf("expected code %s, got %s", ErrCodeProviderUnavailable, err.Code)
	}

	if !err.RetryRecommended {
		t.Error("expected retry to be recommended")
	}
}

func TestNewInvalidInputError(t *testing.T) {
	err := NewInvalidInputError("namespace", "", "namespace name is required")

	if err.Code != ErrCodeInvalidInput {
		t.Errorf("expected code %s, got %s", ErrCodeInvalidInput, err.Code)
	}

	if err.RetryRecommended {
		t.Error("expected retry NOT to be recommended for invalid input")
	}

	if err.Context["field"] != "namespace" {
		t.Errorf("expected field in context, got %v", err.Context["field"])
	}
}

func TestNewAlreadyExistsError(t *testing.T) {
	err := NewAlreadyExistsError("service", "myapp.default")

	if err.Code != ErrCodeAlreadyExists {
		t.Errorf("expected code %s, got %s", ErrCodeAlreadyExists, err.Code)
	}

	if err.RetryRecommended {
		t.Error("expected retry NOT to be recommended")
	}
}

func TestNewNotForwardingError(t *testing.T) {
	err := NewNotForwardingError("myapp.default")

	if err.Code != ErrCodeNotForwarding {
		t.Errorf("expected code %s, got %s", ErrCodeNotForwarding, err.Code)
	}

	if err.RetryRecommended {
		t.Error("expected retry NOT to be recommended")
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		context      map[string]interface{}
		expectedCode string
	}{
		{
			name:         "connection refused",
			err:          errors.New("connection refused to pod"),
			context:      map[string]interface{}{"service": "myapp", "pod": "myapp-123"},
			expectedCode: ErrCodeConnectionRefused,
		},
		{
			name:         "timeout",
			err:          errors.New("operation timed out"),
			context:      map[string]interface{}{"service": "myapp"},
			expectedCode: ErrCodeTimeout,
		},
		{
			name:         "pod not found",
			err:          errors.New("pod not found in namespace"),
			context:      map[string]interface{}{"service": "myapp"},
			expectedCode: ErrCodePodNotFound,
		},
		{
			name:         "service not found",
			err:          errors.New("service not found: myapp"),
			context:      map[string]interface{}{"service": "myapp"},
			expectedCode: ErrCodeServiceNotFound,
		},
		{
			name:         "namespace not found",
			err:          errors.New("namespace not found"),
			context:      map[string]interface{}{"namespace": "staging", "context": "cluster"},
			expectedCode: ErrCodeNamespaceNotFound,
		},
		{
			name:         "permission denied",
			err:          errors.New("permission denied for operation"),
			context:      map[string]interface{}{},
			expectedCode: ErrCodePermissionDenied,
		},
		{
			name:         "forbidden",
			err:          errors.New("forbidden: access denied"),
			context:      map[string]interface{}{},
			expectedCode: ErrCodePermissionDenied,
		},
		{
			name:         "already exists",
			err:          errors.New("service already exists"),
			context:      map[string]interface{}{"service": "myapp"},
			expectedCode: ErrCodeAlreadyExists,
		},
		{
			name:         "unknown error",
			err:          errors.New("something unexpected happened"),
			context:      map[string]interface{}{},
			expectedCode: ErrCodeInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mcpErr := ClassifyError(tt.err, tt.context)
			if mcpErr.Code != tt.expectedCode {
				t.Errorf("expected code %s, got %s", tt.expectedCode, mcpErr.Code)
			}
		})
	}
}

func TestClassifyError_Nil(t *testing.T) {
	result := ClassifyError(nil, nil)
	if result != nil {
		t.Error("expected nil for nil error")
	}
}

func TestExtractServiceName(t *testing.T) {
	tests := []struct {
		key      string
		expected string
	}{
		{"myapp.default.cluster", "myapp"},
		{"myapp.default", "myapp"},
		{"myapp", "myapp"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := extractServiceName(tt.key)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
