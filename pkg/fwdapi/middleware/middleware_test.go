package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func performRequest(r *gin.Engine, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func performRequestWithOrigin(r *gin.Engine, method, path, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Origin", origin)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRecovery(t *testing.T) {
	r := setupRouter()
	r.Use(Recovery())
	r.GET("/panic", func(c *gin.Context) {
		panic("test panic")
	})

	w := performRequest(r, "GET", "/panic")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

func TestRequestLogger(t *testing.T) {
	r := setupRouter()
	r.Use(RequestLogger())
	r.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	w := performRequest(r, "GET", "/test")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestCORS(t *testing.T) {
	r := setupRouter()
	r.Use(CORS())
	r.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// Test with origin header
	w := performRequestWithOrigin(r, "GET", "/test", "http://localhost:3000")

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("Expected Access-Control-Allow-Origin '*', got '%s'",
			w.Header().Get("Access-Control-Allow-Origin"))
	}

	if w.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("Expected Access-Control-Allow-Methods to be set")
	}

	if w.Header().Get("Access-Control-Allow-Headers") == "" {
		t.Error("Expected Access-Control-Allow-Headers to be set")
	}
}

func TestCORS_OptionsRequest(t *testing.T) {
	r := setupRouter()
	r.Use(CORS())

	w := performRequest(r, "OPTIONS", "/test")

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status %d for OPTIONS, got %d", http.StatusNoContent, w.Code)
	}
}

func TestNoCache(t *testing.T) {
	r := setupRouter()
	r.Use(NoCache())
	r.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	w := performRequest(r, "GET", "/test")

	// Check for cache prevention headers (format may vary)
	cacheControl := w.Header().Get("Cache-Control")
	if cacheControl == "" {
		t.Error("Expected Cache-Control header to be set")
	}

	if w.Header().Get("Pragma") != "no-cache" {
		t.Errorf("Expected Pragma to be 'no-cache', got '%s'", w.Header().Get("Pragma"))
	}

	if w.Header().Get("Expires") != "0" {
		t.Errorf("Expected Expires to be '0', got '%s'", w.Header().Get("Expires"))
	}
}

func TestErrorHandler(t *testing.T) {
	r := setupRouter()
	r.Use(ErrorHandler())
	r.GET("/error", func(c *gin.Context) {
		c.AbortWithError(http.StatusBadRequest, http.ErrBodyNotAllowed)
	})

	w := performRequest(r, "GET", "/error")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestErrorHandler_NoError(t *testing.T) {
	r := setupRouter()
	r.Use(ErrorHandler())
	r.GET("/ok", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	w := performRequest(r, "GET", "/ok")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestMiddlewareChain(t *testing.T) {
	r := setupRouter()
	r.Use(Recovery())
	r.Use(RequestLogger())
	r.Use(CORS())
	r.Use(NoCache())
	r.Use(ErrorHandler())
	r.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	w := performRequest(r, "GET", "/test")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Verify all headers are set
	if w.Header().Get("Cache-Control") == "" {
		t.Error("Expected Cache-Control header to be set")
	}
}

func TestRequestLogger_WithQueryString(t *testing.T) {
	r := setupRouter()
	r.Use(RequestLogger())
	r.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// Perform request with query string
	req := httptest.NewRequest("GET", "/test?foo=bar&baz=qux", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestErrorHandler_DefaultsTo500(t *testing.T) {
	r := setupRouter()
	r.Use(ErrorHandler())
	r.GET("/error-default", func(c *gin.Context) {
		// Add error without changing status (will default to 500)
		_ = c.Error(http.ErrBodyNotAllowed)
	})

	w := performRequest(r, "GET", "/error-default")

	// Status should be 500 since we didn't set one explicitly
	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}
