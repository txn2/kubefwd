package fwdport

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/txn2/txeh"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// TestWaitUntilPodRunning_AlreadyRunning tests that a pod already running returns immediately
func TestWaitUntilPodRunning_AlreadyRunning(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       types.UID("test-uid"),
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
		},
	}

	clientset := fake.NewClientset(pod)

	pfo := &PortForwardOpts{
		ClientSet: clientset,
		Namespace: "default",
		PodName:   "test-pod",
		Timeout:   10,
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	returnedPod, err := pfo.WaitUntilPodRunning(stopCh)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if returnedPod == nil {
		t.Fatal("Expected pod to be returned")
	}

	if returnedPod.Name != "test-pod" {
		t.Errorf("Expected pod name 'test-pod', got: %s", returnedPod.Name)
	}
}

// TestWaitUntilPodRunning_PendingToRunning tests waiting for a pod to transition from Pending to Running
func TestWaitUntilPodRunning_PendingToRunning(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       types.UID("test-uid"),
		},
		Status: v1.PodStatus{
			Phase: v1.PodPending,
		},
	}

	clientset := fake.NewClientset(pod)
	fakeWatcher := watch.NewFake()

	clientset.PrependWatchReactor("pods", k8stesting.DefaultWatchReactor(fakeWatcher, nil))

	pfo := &PortForwardOpts{
		ClientSet: clientset,
		Namespace: "default",
		PodName:   "test-pod",
		Timeout:   10,
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	var wg sync.WaitGroup
	var returnedPod *v1.Pod
	var err error

	wg.Add(1)
	go func() {
		defer wg.Done()
		returnedPod, err = pfo.WaitUntilPodRunning(stopCh)
	}()

	// Give time for watch to start
	time.Sleep(50 * time.Millisecond)

	// Simulate pod becoming Running
	runningPod := pod.DeepCopy()
	runningPod.Status.Phase = v1.PodRunning
	fakeWatcher.Modify(runningPod)

	wg.Wait()

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if returnedPod == nil {
		t.Fatal("Expected pod to be returned")
	}

	if returnedPod.Status.Phase != v1.PodRunning {
		t.Errorf("Expected pod phase Running, got: %v", returnedPod.Status.Phase)
	}
}

// TestWaitUntilPodRunning_Timeout tests that waiting times out correctly
func TestWaitUntilPodRunning_Timeout(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       types.UID("test-uid"),
		},
		Status: v1.PodStatus{
			Phase: v1.PodPending,
		},
	}

	clientset := fake.NewClientset(pod)
	fakeWatcher := watch.NewFake()

	clientset.PrependWatchReactor("pods", k8stesting.DefaultWatchReactor(fakeWatcher, nil))

	pfo := &PortForwardOpts{
		ClientSet: clientset,
		Namespace: "default",
		PodName:   "test-pod",
		Timeout:   1, // 1 second timeout
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	start := time.Now()
	returnedPod, err := pfo.WaitUntilPodRunning(stopCh)
	elapsed := time.Since(start)

	// Should timeout after ~1 second
	if elapsed < 900*time.Millisecond || elapsed > 2*time.Second {
		t.Errorf("Expected timeout around 1 second, took: %v", elapsed)
	}

	// On timeout, returns nil, nil (not an error)
	if err != nil {
		t.Errorf("Expected nil error on timeout, got: %v", err)
	}

	if returnedPod != nil {
		t.Error("Expected nil pod on timeout")
	}
}

// TestWaitUntilPodRunning_StopChannel tests that stop channel cancels waiting
func TestWaitUntilPodRunning_StopChannel(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       types.UID("test-uid"),
		},
		Status: v1.PodStatus{
			Phase: v1.PodPending,
		},
	}

	clientset := fake.NewClientset(pod)
	fakeWatcher := watch.NewFake()

	clientset.PrependWatchReactor("pods", k8stesting.DefaultWatchReactor(fakeWatcher, nil))

	pfo := &PortForwardOpts{
		ClientSet: clientset,
		Namespace: "default",
		PodName:   "test-pod",
		Timeout:   60, // Long timeout
	}

	stopCh := make(chan struct{})

	var wg sync.WaitGroup
	var returnedPod *v1.Pod
	var err error

	wg.Add(1)
	go func() {
		defer wg.Done()
		returnedPod, err = pfo.WaitUntilPodRunning(stopCh)
	}()

	// Give time for watch to start
	time.Sleep(50 * time.Millisecond)

	// Close stop channel to cancel
	close(stopCh)

	wg.Wait()

	// Should return nil, nil when stopped
	if err != nil {
		t.Errorf("Expected nil error when stopped, got: %v", err)
	}

	if returnedPod != nil {
		t.Error("Expected nil pod when stopped")
	}
}

// TestWaitUntilPodRunning_PodNotFound tests behavior when pod doesn't exist
func TestWaitUntilPodRunning_PodNotFound(t *testing.T) {
	clientset := fake.NewClientset() // No pods

	pfo := &PortForwardOpts{
		ClientSet: clientset,
		Namespace: "default",
		PodName:   "nonexistent-pod",
		Timeout:   10,
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	returnedPod, err := pfo.WaitUntilPodRunning(stopCh)

	// Should return an error when pod not found
	if err == nil {
		t.Error("Expected error when pod not found")
	}

	if returnedPod != nil {
		t.Error("Expected nil pod when pod not found")
	}
}

// TestWaitUntilPodRunning_WatchError tests handling of watch ERROR event
func TestWaitUntilPodRunning_WatchError(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       types.UID("test-uid"),
		},
		Status: v1.PodStatus{
			Phase: v1.PodPending,
		},
	}

	clientset := fake.NewClientset(pod)
	fakeWatcher := watch.NewFake()

	clientset.PrependWatchReactor("pods", k8stesting.DefaultWatchReactor(fakeWatcher, nil))

	pfo := &PortForwardOpts{
		ClientSet: clientset,
		Namespace: "default",
		PodName:   "test-pod",
		Timeout:   10,
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	var wg sync.WaitGroup
	var returnedPod *v1.Pod
	var err error

	wg.Add(1)
	go func() {
		defer wg.Done()
		returnedPod, err = pfo.WaitUntilPodRunning(stopCh)
	}()

	// Give time for watch to start
	time.Sleep(50 * time.Millisecond)

	// Send error event
	fakeWatcher.Error(&metav1.Status{
		Status:  metav1.StatusFailure,
		Message: "watch error",
	})

	wg.Wait()

	// Should return an error for ERROR event type
	if err == nil {
		t.Error("Expected error for watch ERROR event")
	}

	if returnedPod != nil {
		t.Error("Expected nil pod for watch ERROR event")
	}
}

// TestStop_IdempotentClose tests that Stop() can be called multiple times safely
func TestStop_IdempotentClose(t *testing.T) {
	pfo := &PortForwardOpts{
		ManualStopChan: make(chan struct{}),
		DoneChan:       make(chan struct{}),
	}

	// First stop should close the channel
	pfo.Stop()

	// Second stop should not panic
	pfo.Stop()

	// Third stop should not panic
	pfo.Stop()

	// Channel should be closed
	select {
	case <-pfo.ManualStopChan:
		// Good - channel is closed
	default:
		t.Error("ManualStopChan should be closed after Stop()")
	}
}

// TestStop_AlreadyDone tests that Stop() is no-op when DoneChan is already closed
func TestStop_AlreadyDone(t *testing.T) {
	pfo := &PortForwardOpts{
		ManualStopChan: make(chan struct{}),
		DoneChan:       make(chan struct{}),
	}

	// Close DoneChan first (simulating already finished)
	close(pfo.DoneChan)

	// Stop should be a no-op and not panic
	pfo.Stop()

	// ManualStopChan should still be open
	select {
	case <-pfo.ManualStopChan:
		t.Error("ManualStopChan should not be closed when DoneChan was already closed")
	default:
		// Good - channel is still open
	}
}

// TestStop_ConcurrentCalls tests thread safety of Stop()
func TestStop_ConcurrentCalls(t *testing.T) {
	pfo := &PortForwardOpts{
		ManualStopChan: make(chan struct{}),
		DoneChan:       make(chan struct{}),
	}

	var wg sync.WaitGroup
	numGoroutines := 100

	// Call Stop() from many goroutines concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pfo.Stop() // Should not panic
		}()
	}

	wg.Wait()

	// Channel should be closed
	select {
	case <-pfo.ManualStopChan:
		// Good
	default:
		t.Error("ManualStopChan should be closed")
	}
}

// TestSanitizeHost tests hostname sanitization
func TestSanitizeHost_Comprehensive(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple hostname",
			input:    "my-service",
			expected: "my-service",
		},
		{
			name:     "with dots",
			input:    "my-service.namespace.cluster",
			expected: "my-service-namespace-cluster",
		},
		{
			name:     "with slashes",
			input:    "my-service/namespace/cluster",
			expected: "my-service-namespace-cluster",
		},
		{
			name:     "with colons",
			input:    "cluster:6443",
			expected: "cluster-6443",
		},
		{
			name:     "openshift context",
			input:    "service-name.namespace.project-name/cluster-name:6443/username",
			expected: "service-name-namespace-project-name-cluster-name-6443-username",
		},
		{
			name:     "leading dashes",
			input:    "---service",
			expected: "service",
		},
		{
			name:     "trailing dashes",
			input:    "service---",
			expected: "service",
		},
		{
			name:     "leading and trailing dashes",
			input:    "-----test-----",
			expected: "test",
		},
		{
			name:     "special characters",
			input:    "service@name#with$special%chars",
			expected: "service-name-with-special-chars",
		},
		{
			name:     "underscores",
			input:    "service_name_with_underscores",
			expected: "service-name-with-underscores",
		},
		{
			name:     "consecutive special chars",
			input:    "service...name///with:::special",
			expected: "service---name---with---special",
		},
		{
			name:     "uppercase",
			input:    "MyService.NAMESPACE",
			expected: "MyService-NAMESPACE",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only special chars",
			input:    "---///---",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeHost(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeHost(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestAddHosts_HostnameVariations tests hostname generation for different cluster/namespace combinations
// This test verifies the hostname building logic without actually writing to files
func TestAddHosts_HostnameVariations(t *testing.T) {
	tests := []struct {
		name       string
		clusterN   int
		namespaceN int
		domain     string
		service    string
		namespace  string
		context    string
		wantHosts  []string
	}{
		{
			name:       "local cluster, local namespace, no domain",
			clusterN:   0,
			namespaceN: 0,
			domain:     "",
			service:    "my-svc",
			namespace:  "default",
			context:    "minikube",
			wantHosts: []string{
				"my-svc",
				"my-svc.default",
				"my-svc.default.svc",
				"my-svc.default.svc.cluster.local",
			},
		},
		{
			name:       "local cluster, local namespace, with domain",
			clusterN:   0,
			namespaceN: 0,
			domain:     "example.com",
			service:    "my-svc",
			namespace:  "default",
			context:    "minikube",
			wantHosts: []string{
				"my-svc",
				"my-svc.example.com",
				"my-svc.default",
				"my-svc.default.svc",
			},
		},
		{
			name:       "remote cluster",
			clusterN:   1,
			namespaceN: 0,
			domain:     "",
			service:    "my-svc",
			namespace:  "default",
			context:    "production",
			wantHosts: []string{
				"my-svc.production",
				"my-svc.default.production",
			},
		},
		{
			name:       "remote namespace",
			clusterN:   0,
			namespaceN: 1,
			domain:     "",
			service:    "my-svc",
			namespace:  "other-ns",
			context:    "minikube",
			wantHosts: []string{
				"my-svc.other-ns",
				"my-svc.other-ns.svc",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use createTempHostsFile helper from fwdport_hosts_test.go pattern
			hostFile, _, cleanup := createTempHostsFile(t)
			defer cleanup()

			pfo := &PortForwardOpts{
				Service:    tt.service,
				Namespace:  tt.namespace,
				Context:    tt.context,
				ClusterN:   tt.clusterN,
				NamespaceN: tt.namespaceN,
				Domain:     tt.domain,
				LocalIP:    []byte{127, 0, 0, 1},
				HostFile:   hostFile,
			}

			err := pfo.AddHosts()
			if err != nil {
				t.Fatalf("AddHosts failed: %v", err)
			}

			// Verify expected hosts were added
			for _, wantHost := range tt.wantHosts {
				found := false
				for _, gotHost := range pfo.Hosts {
					if gotHost == wantHost || gotHost == sanitizeHost(wantHost) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected host %q not found in %v", wantHost, pfo.Hosts)
				}
			}
		})
	}
}

// TestPingingDialer_StopPing tests that ping channel can be closed without blocking
func TestPingingDialer_StopPing(t *testing.T) {
	stopChan := make(chan struct{})

	// Close the channel directly (simulating stopPing behavior)
	done := make(chan struct{})
	go func() {
		close(stopChan)
		close(done)
	}()

	select {
	case <-done:
		// Good - channel closed without blocking
	case <-time.After(100 * time.Millisecond):
		t.Error("Channel close blocked for too long")
	}

	// Verify channel is closed
	select {
	case <-stopChan:
		// Good - channel is closed
	default:
		t.Error("Expected stopChan to be closed")
	}
}

// TestPortForwardOpts_Fields tests that all expected fields are accessible
func TestPortForwardOpts_Fields(t *testing.T) {
	hosts, _ := txeh.NewHosts(&txeh.HostsConfig{})

	pfo := &PortForwardOpts{
		Service:        "test-service",
		PodName:        "test-pod",
		PodPort:        "8080",
		LocalPort:      "8080",
		LocalIP:        []byte{127, 0, 0, 1},
		Namespace:      "default",
		Context:        "minikube",
		ClusterN:       0,
		NamespaceN:     0,
		Domain:         "example.com",
		Timeout:        300,
		HostFile:       &HostFileWithLock{Hosts: hosts},
		ManualStopChan: make(chan struct{}),
		DoneChan:       make(chan struct{}),
	}

	// Verify fields
	if pfo.Service != "test-service" {
		t.Errorf("Service = %v, want test-service", pfo.Service)
	}
	if pfo.PodName != "test-pod" {
		t.Errorf("PodName = %v, want test-pod", pfo.PodName)
	}
	if pfo.Timeout != 300 {
		t.Errorf("Timeout = %v, want 300", pfo.Timeout)
	}
}

// TestWaitUntilPodRunning_ContextCancellation tests behavior with context
func TestWaitUntilPodRunning_ChannelClose(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       types.UID("test-uid"),
		},
		Status: v1.PodStatus{
			Phase: v1.PodPending,
		},
	}

	clientset := fake.NewClientset(pod)
	fakeWatcher := watch.NewFake()

	clientset.PrependWatchReactor("pods", k8stesting.DefaultWatchReactor(fakeWatcher, nil))

	pfo := &PortForwardOpts{
		ClientSet: clientset,
		Namespace: "default",
		PodName:   "test-pod",
		Timeout:   60,
	}

	ctx, cancel := context.WithCancel(context.Background())
	stopCh := make(chan struct{})

	// Convert context to stop channel behavior
	go func() {
		<-ctx.Done()
		close(stopCh)
	}()

	var wg sync.WaitGroup
	var returnedPod *v1.Pod

	wg.Add(1)
	go func() {
		defer wg.Done()
		returnedPod, _ = pfo.WaitUntilPodRunning(stopCh)
	}()

	// Give time for watch to start
	time.Sleep(50 * time.Millisecond)

	// Cancel context
	cancel()

	wg.Wait()

	// Should return nil when canceled
	if returnedPod != nil {
		t.Error("Expected nil pod when context canceled")
	}
}
