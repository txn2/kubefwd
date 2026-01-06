package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDocsHandler_Docs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	handler := NewDocsHandler("1.22.0")
	router.GET("/docs", handler.Docs)

	req := httptest.NewRequest(http.MethodGet, "/docs", http.NoBody)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("Expected Content-Type text/html, got %s", contentType)
	}

	body := w.Body.String()
	if !strings.Contains(body, "redoc") {
		t.Error("Expected body to contain 'redoc'")
	}

	if !strings.Contains(body, "kubefwd API Documentation") {
		t.Error("Expected body to contain 'kubefwd API Documentation'")
	}

	if !strings.Contains(body, "/openapi.yaml") {
		t.Error("Expected body to contain '/openapi.yaml' spec URL")
	}
}

func TestDocsHandler_OpenAPISpec(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	handler := NewDocsHandler("1.22.0")
	router.GET("/openapi.yaml", handler.OpenAPISpec)

	req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", http.NoBody)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/x-yaml" {
		t.Errorf("Expected Content-Type application/x-yaml, got %s", contentType)
	}

	// Check CORS header is set
	corsHeader := w.Header().Get("Access-Control-Allow-Origin")
	if corsHeader != "*" {
		t.Errorf("Expected CORS header '*', got '%s'", corsHeader)
	}

	body := w.Body.String()
	if !strings.Contains(body, "openapi:") {
		t.Error("Expected body to contain 'openapi:' YAML key")
	}

	if !strings.Contains(body, "kubefwd API") {
		t.Error("Expected body to contain 'kubefwd API'")
	}

	// Verify it contains key endpoints
	if !strings.Contains(body, "/v1/services") {
		t.Error("Expected spec to document /v1/services endpoint")
	}

	if !strings.Contains(body, "/v1/namespaces") {
		t.Error("Expected spec to document /v1/namespaces endpoint")
	}
}

func TestDocsHandler_VersionInjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Use a specific version to verify injection
	testVersion := "1.23.456"
	handler := NewDocsHandler(testVersion)
	router.GET("/openapi.yaml", handler.OpenAPISpec)

	req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", http.NoBody)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	body := w.Body.String()

	// Should contain the injected version
	if !strings.Contains(body, `version: "1.23.456"`) {
		t.Error("Expected spec to contain injected version '1.23.456'")
	}

	// Should NOT contain the placeholder version
	if strings.Contains(body, `version: "0.0.0"`) {
		t.Error("Spec should not contain placeholder version '0.0.0'")
	}
}

func TestDocsHandler_EmbeddedSpecNotEmpty(t *testing.T) {
	if len(openapiSpec) == 0 {
		t.Error("Embedded OpenAPI spec should not be empty")
	}

	// Should be valid YAML starting with openapi version
	specStr := string(openapiSpec)
	if !strings.HasPrefix(specStr, "openapi:") {
		t.Error("OpenAPI spec should start with 'openapi:' key")
	}

	// Embedded spec should have placeholder version
	if !strings.Contains(specStr, `version: "0.0.0"`) {
		t.Error("Embedded spec should contain placeholder version '0.0.0'")
	}
}
