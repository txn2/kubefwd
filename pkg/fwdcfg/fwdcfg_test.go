package fwdcfg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewConfigGetter(t *testing.T) {
	cg := NewConfigGetter()
	if cg == nil {
		t.Fatal("NewConfigGetter returned nil")
	}
	if cg.ConfigFlag == nil {
		t.Error("ConfigFlag is nil")
	}
}

func TestGetClientConfig_WithValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	kubeconfig := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://localhost:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: test-token
`
	if err := os.WriteFile(configPath, []byte(kubeconfig), 0o644); err != nil {
		t.Fatalf("Failed to write kubeconfig: %v", err)
	}

	cg := NewConfigGetter()
	config, err := cg.GetClientConfig(configPath)
	if err != nil {
		t.Fatalf("GetClientConfig failed: %v", err)
	}

	if config.CurrentContext != "test-context" {
		t.Errorf("Expected current-context 'test-context', got %s", config.CurrentContext)
	}

	if len(config.Clusters) != 1 {
		t.Errorf("Expected 1 cluster, got %d", len(config.Clusters))
	}

	if len(config.Contexts) != 1 {
		t.Errorf("Expected 1 context, got %d", len(config.Contexts))
	}
}

func TestGetClientConfig_WithInvalidPath(t *testing.T) {
	cg := NewConfigGetter()
	_, err := cg.GetClientConfig("/nonexistent/path/config")
	if err == nil {
		t.Error("Expected error for nonexistent config path")
	}
}

func TestGetClientConfig_EmptyPath(t *testing.T) {
	// Set up a valid KUBECONFIG for the test
	originalKubeconfig := os.Getenv("KUBECONFIG")
	defer func() { _ = os.Setenv("KUBECONFIG", originalKubeconfig) }()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")
	kubeconfig := `apiVersion: v1
kind: Config
current-context: env-context
contexts:
- context:
    cluster: env-cluster
  name: env-context
clusters:
- cluster:
    server: https://localhost:6443
  name: env-cluster
`
	if err := os.WriteFile(configPath, []byte(kubeconfig), 0o644); err != nil {
		t.Fatalf("Failed to write kubeconfig: %v", err)
	}
	if err := os.Setenv("KUBECONFIG", configPath); err != nil {
		t.Fatalf("Failed to set KUBECONFIG: %v", err)
	}

	cg := NewConfigGetter()
	config, err := cg.GetClientConfig("")
	if err != nil {
		t.Fatalf("GetClientConfig with empty path failed: %v", err)
	}

	if config.CurrentContext != "env-context" {
		t.Errorf("Expected 'env-context', got %s", config.CurrentContext)
	}
}

func TestGetRestConfig_ValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	kubeconfig := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://localhost:6443
    insecure-skip-tls-verify: true
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: test-token
`
	if err := os.WriteFile(configPath, []byte(kubeconfig), 0o644); err != nil {
		t.Fatalf("Failed to write kubeconfig: %v", err)
	}

	cg := NewConfigGetter()
	restConfig, err := cg.GetRestConfig(configPath, "test-context")
	if err != nil {
		t.Fatalf("GetRestConfig failed: %v", err)
	}

	if restConfig.Host != "https://localhost:6443" {
		t.Errorf("Expected host 'https://localhost:6443', got %s", restConfig.Host)
	}
}

func TestGetRestConfig_InvalidContext(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	kubeconfig := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://localhost:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: test-token
`
	if err := os.WriteFile(configPath, []byte(kubeconfig), 0o644); err != nil {
		t.Fatalf("Failed to write kubeconfig: %v", err)
	}

	cg := NewConfigGetter()
	_, err := cg.GetRestConfig(configPath, "nonexistent-context")
	if err == nil {
		t.Error("Expected error for nonexistent context")
	}
}

func TestGetClientConfig_MalformedYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	malformedYAML := `apiVersion: v1
kind: Config
clusters: [invalid yaml structure
`
	if err := os.WriteFile(configPath, []byte(malformedYAML), 0o644); err != nil {
		t.Fatalf("Failed to write malformed kubeconfig: %v", err)
	}

	cg := NewConfigGetter()
	_, err := cg.GetClientConfig(configPath)
	if err == nil {
		t.Error("Expected error for malformed YAML")
	}
}

func TestGetClientConfig_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	if err := os.WriteFile(configPath, []byte(""), 0o644); err != nil {
		t.Fatalf("Failed to write empty kubeconfig: %v", err)
	}

	cg := NewConfigGetter()
	config, err := cg.GetClientConfig(configPath)
	// Empty file should parse but result in empty config
	if err != nil {
		t.Fatalf("GetClientConfig with empty file failed: %v", err)
	}

	if config.CurrentContext != "" {
		t.Errorf("Expected empty current-context, got %s", config.CurrentContext)
	}
}

func TestGetRestConfig_EmptyContext(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	kubeconfig := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://localhost:6443
    insecure-skip-tls-verify: true
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: test-token
`
	if err := os.WriteFile(configPath, []byte(kubeconfig), 0o644); err != nil {
		t.Fatalf("Failed to write kubeconfig: %v", err)
	}

	cg := NewConfigGetter()
	// Empty context should use current-context from the file
	restConfig, err := cg.GetRestConfig(configPath, "")
	if err != nil {
		t.Fatalf("GetRestConfig with empty context failed: %v", err)
	}

	if restConfig.Host != "https://localhost:6443" {
		t.Errorf("Expected host 'https://localhost:6443', got %s", restConfig.Host)
	}
}

func TestGetCurrentContext(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	kubeconfig := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://localhost:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: my-current-context
current-context: my-current-context
users:
- name: test-user
  user:
    token: test-token
`
	if err := os.WriteFile(configPath, []byte(kubeconfig), 0o644); err != nil {
		t.Fatalf("Failed to write kubeconfig: %v", err)
	}

	cg := NewConfigGetter()
	ctx, err := cg.GetCurrentContext(configPath)
	if err != nil {
		t.Fatalf("GetCurrentContext failed: %v", err)
	}

	if ctx != "my-current-context" {
		t.Errorf("Expected 'my-current-context', got %s", ctx)
	}
}

func TestGetCurrentContext_InvalidPath(t *testing.T) {
	cg := NewConfigGetter()
	_, err := cg.GetCurrentContext("/nonexistent/path/config")
	if err == nil {
		t.Error("Expected error for nonexistent config path")
	}
}

func TestGetCurrentContext_EmptyConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	kubeconfig := `apiVersion: v1
kind: Config
`
	if err := os.WriteFile(configPath, []byte(kubeconfig), 0o644); err != nil {
		t.Fatalf("Failed to write kubeconfig: %v", err)
	}

	cg := NewConfigGetter()
	ctx, err := cg.GetCurrentContext(configPath)
	if err != nil {
		t.Fatalf("GetCurrentContext failed: %v", err)
	}

	if ctx != "" {
		t.Errorf("Expected empty context, got %s", ctx)
	}
}

func TestGetRESTClient_NoCluster(t *testing.T) {
	// Set up a kubeconfig with invalid cluster
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	kubeconfig := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://nonexistent-cluster:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: test-token
`
	if err := os.WriteFile(configPath, []byte(kubeconfig), 0o644); err != nil {
		t.Fatalf("Failed to write kubeconfig: %v", err)
	}

	// Set environment variable to use our test config
	originalKubeconfig := os.Getenv("KUBECONFIG")
	defer func() { _ = os.Setenv("KUBECONFIG", originalKubeconfig) }()
	if err := os.Setenv("KUBECONFIG", configPath); err != nil {
		t.Fatalf("Failed to set KUBECONFIG: %v", err)
	}

	cg := NewConfigGetter()
	_, err := cg.GetRESTClient()
	// This will fail because the cluster is not reachable
	// but the function should execute without panicking
	if err != nil {
		// Expected - cluster is unreachable
		t.Logf("GetRESTClient returned expected error: %v", err)
	} else {
		// If somehow it succeeded, that's fine too
		t.Log("GetRESTClient succeeded (cluster may be reachable)")
	}
}

func TestGetRESTClient_NoConfig(t *testing.T) {
	// Set up a kubeconfig with no context
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	kubeconfig := `apiVersion: v1
kind: Config
`
	if err := os.WriteFile(configPath, []byte(kubeconfig), 0o644); err != nil {
		t.Fatalf("Failed to write kubeconfig: %v", err)
	}

	originalKubeconfig := os.Getenv("KUBECONFIG")
	defer func() { _ = os.Setenv("KUBECONFIG", originalKubeconfig) }()
	if err := os.Setenv("KUBECONFIG", configPath); err != nil {
		t.Fatalf("Failed to set KUBECONFIG: %v", err)
	}

	cg := NewConfigGetter()
	_, err := cg.GetRESTClient()
	// The function may succeed or fail depending on in-cluster config availability
	// What's important is that it doesn't panic and returns either a valid client or an error
	if err != nil {
		t.Logf("GetRESTClient returned expected error: %v", err)
	} else {
		t.Log("GetRESTClient succeeded (in-cluster config may be available)")
	}
}
