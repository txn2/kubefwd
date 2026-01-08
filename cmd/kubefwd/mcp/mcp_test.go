package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifyAPIConnection_Success(t *testing.T) {
	// Create a mock server that returns a healthy response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"healthy"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	err := verifyAPIConnection(server.URL)
	if err != nil {
		t.Errorf("verifyAPIConnection() returned error for healthy server: %v", err)
	}
}

func TestVerifyAPIConnection_ServerDown(t *testing.T) {
	// Use an invalid URL that won't connect
	err := verifyAPIConnection("http://localhost:59999")
	if err == nil {
		t.Error("verifyAPIConnection() should return error when server is unreachable")
	}
}

func TestVerifyAPIConnection_ServerError(t *testing.T) {
	// Create a mock server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	err := verifyAPIConnection(server.URL)
	if err == nil {
		t.Error("verifyAPIConnection() should return error when server returns 500")
	}
}

func TestVerifyAPIConnection_InvalidJSON(t *testing.T) {
	// Create a mock server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not json`))
	}))
	defer server.Close()

	err := verifyAPIConnection(server.URL)
	if err == nil {
		t.Error("verifyAPIConnection() should return error for invalid JSON response")
	}
}

func TestCmd_FlagsExist(t *testing.T) {
	// Verify the command has the expected flags
	apiURLFlag := Cmd.Flags().Lookup("api-url")
	if apiURLFlag == nil {
		t.Fatal("Expected --api-url flag to exist")
	}
	if apiURLFlag.DefValue != "http://kubefwd.internal/api" {
		t.Errorf("Expected --api-url default to be 'http://kubefwd.internal/api', got %s", apiURLFlag.DefValue)
	}

	verboseFlag := Cmd.Flags().Lookup("verbose")
	if verboseFlag == nil {
		t.Fatal("Expected --verbose flag to exist")
	}
	if verboseFlag.Shorthand != "v" {
		t.Errorf("Expected --verbose shorthand to be 'v', got %s", verboseFlag.Shorthand)
	}
}

func TestCmd_Metadata(t *testing.T) {
	if Cmd.Use != "mcp" {
		t.Errorf("Expected Cmd.Use to be 'mcp', got %s", Cmd.Use)
	}

	if Cmd.Short == "" {
		t.Error("Expected Cmd.Short to be non-empty")
	}

	if Cmd.Long == "" {
		t.Error("Expected Cmd.Long to be non-empty")
	}

	if Cmd.Example == "" {
		t.Error("Expected Cmd.Example to be non-empty")
	}
}
