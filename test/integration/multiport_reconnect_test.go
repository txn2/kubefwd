//go:build integration
// +build integration

package integration

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"
)

// apiBase is the kubefwd REST API base URL. The API server binds a
// dedicated loopback IP (see pkg/fwdapi/manager.go: APIIP/APIPort) and is
// enabled with --api.
const apiBase = "http://127.2.27.1/api"

// TestMultiPortReconnect is the end-to-end regression test for issue #509.
//
// A multi-port normal service must keep ALL of its ports forwarded across a
// resync. Previously syncNormalService kept a single forward key and dropped
// the rest, so a resync (the path auto-reconnect drives after a single port's
// connection is reset) left the service unable to recover the dropped port and
// removed its shared /etc/hosts entry.
//
// Reproducing the exact upstream RST trigger (kubernetes/kubernetes#111825) is
// not deterministic, so this test drives the same code path deterministically:
// it forces a resync via the REST API (POST /v1/services/:key/sync?force=true,
// which calls SyncPodForwards(true)) and asserts every port still serves
// afterwards. On the pre-fix code this forced sync stopped one port and removed
// the `multiport` hostname, breaking both ports.
//
//goland:noinspection DuplicatedCode
func TestMultiPortReconnect(t *testing.T) {
	requiresSudo(t)
	requiresKindCluster(t)

	// Use a known API key so the test can authenticate. startKubefwd copies
	// os.Environ() into the child process, so set it before starting.
	const apiKey = "kubefwd-integration-test-key"
	t.Setenv("KUBEFWD_API_KEY", apiKey)

	// Start kubefwd with auto-reconnect and the REST API enabled.
	cmd := startKubefwd(t, "svc", "-n", "test-multiport", "-a", "--api", "-v")
	defer stopKubefwd(t, cmd)

	t.Log("Waiting for initial forwarding to stabilize...")
	time.Sleep(10 * time.Second)

	// Both ports must serve before we do anything.
	assertPortServes(t, "http://multiport:80/", "ok-80", "before resync")
	assertPortServes(t, "http://multiport:8080/", "ok-8080", "before resync")
	t.Log("✓ Both ports serving before resync")

	// Discover the registry key for the multiport service via the API.
	key := discoverServiceKey(t, apiKey, "multiport.test-multiport.")
	t.Logf("Service key: %s", key)

	// Force a resync — the exact code path auto-reconnect drives. On pre-fix
	// code this drops one port and removes the shared hostname.
	t.Log("Forcing resync via API...")
	forceSync(t, apiKey, key)

	// Give the async SyncPodForwards time to run.
	time.Sleep(8 * time.Second)

	// The crux of #509: after the resync BOTH ports must still serve.
	assertPortServes(t, "http://multiport:80/", "ok-80", "after resync (#509)")
	assertPortServes(t, "http://multiport:8080/", "ok-8080", "after resync (#509)")
	t.Log("✓ Both ports still serving after resync — #509 fixed")
}

// assertPortServes fails the test if the URL does not return HTTP 200 with the
// expected body substring. A removed /etc/hosts entry surfaces here as a DNS
// lookup failure.
func assertPortServes(t *testing.T, url, wantBody, phase string) {
	t.Helper()

	resp, err := httpGet(url, 5, 1*time.Second)
	if err != nil {
		t.Fatalf("[%s] %s did not respond: %v", phase, url, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("[%s] %s returned HTTP %d (expected 200)", phase, url, resp.StatusCode)
	}
	if !strings.Contains(string(body), wantBody) {
		t.Fatalf("[%s] %s body %q does not contain %q", phase, url, string(body), wantBody)
	}
}

// discoverServiceKey queries the API service list and returns the first
// registry key with the given prefix.
func discoverServiceKey(t *testing.T, apiKey, prefix string) string {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, apiBase+"/v1/services", nil)
	if err != nil {
		t.Fatalf("failed to build services request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to list services via API: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("services list returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	re := regexp.MustCompile(regexp.QuoteMeta(prefix) + `[A-Za-z0-9._-]+`)
	key := re.FindString(string(body))
	if key == "" {
		t.Fatalf("no service key with prefix %q found in API response: %s", prefix, string(body))
	}
	return key
}

// forceSync issues a forced resync for the given service key.
func forceSync(t *testing.T, apiKey, key string) {
	t.Helper()

	url := fmt.Sprintf("%s/v1/services/%s/sync?force=true", apiBase, key)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		t.Fatalf("failed to build sync request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to call sync endpoint: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		t.Fatalf("sync endpoint returned HTTP %d: %s", resp.StatusCode, string(body))
	}
}
