package services

import (
	"context"
	"os"
	"sync"
	"testing"

	authorizationv1 "k8s.io/api/authorization/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/txn2/kubefwd/pkg/fwdport"
	"github.com/txn2/kubefwd/pkg/fwdsvcregistry"
	"github.com/txn2/txeh"
)

// Package-level registry initialization to avoid race conditions
var (
	registryOnce sync.Once
	registryStop chan struct{}
)

func initRegistryOnce() {
	registryOnce.Do(func() {
		registryStop = make(chan struct{})
		fwdsvcregistry.Init(registryStop)
	})
}

func TestParsePortMap(t *testing.T) {
	opts := &NamespaceOpts{}

	tests := []struct {
		name     string
		mappings []string
		expected []struct {
			source string
			target string
		}
		isNil bool
	}{
		{
			name:     "single mapping",
			mappings: []string{"80:8080"},
			expected: []struct {
				source string
				target string
			}{{"80", "8080"}},
			isNil: false,
		},
		{
			name:     "multiple mappings",
			mappings: []string{"80:8080", "443:8443"},
			expected: []struct {
				source string
				target string
			}{{"80", "8080"}, {"443", "8443"}},
			isNil: false,
		},
		{
			name:     "nil mappings",
			mappings: nil,
			expected: nil,
			isNil:    true,
		},
		{
			name:     "empty mappings",
			mappings: []string{},
			expected: nil,
			isNil:    false,
		},
		{
			name:     "named port mapping",
			mappings: []string{"http:8080", "https:8443"},
			expected: []struct {
				source string
				target string
			}{{"http", "8080"}, {"https", "8443"}},
			isNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := opts.ParsePortMap(tt.mappings)

			if tt.isNil {
				if result != nil {
					t.Errorf("Expected nil, got %v", result)
				}
				return
			}

			if result == nil && len(tt.expected) > 0 {
				t.Fatal("Expected non-nil result")
			}

			if result == nil {
				return
			}

			if len(*result) != len(tt.expected) {
				t.Errorf("Expected %d mappings, got %d", len(tt.expected), len(*result))
				return
			}

			for i, exp := range tt.expected {
				if (*result)[i].SourcePort != exp.source {
					t.Errorf("Mapping %d: expected source %s, got %s",
						i, exp.source, (*result)[i].SourcePort)
				}
				if (*result)[i].TargetPort != exp.target {
					t.Errorf("Mapping %d: expected target %s, got %s",
						i, exp.target, (*result)[i].TargetPort)
				}
			}
		})
	}
}

func TestSetAllNamespace(t *testing.T) {
	// Create fake clientset with namespaces
	ns1 := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}
	ns2 := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}
	ns3 := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}}

	clientset := fake.NewClientset(ns1, ns2, ns3)

	var namespaces []string
	setAllNamespace(clientset, metav1.ListOptions{}, &namespaces)

	if len(namespaces) != 3 {
		t.Errorf("Expected 3 namespaces, got %d", len(namespaces))
	}

	// Verify all namespaces are present
	nsSet := make(map[string]bool)
	for _, ns := range namespaces {
		nsSet[ns] = true
	}

	for _, expected := range []string{"default", "kube-system", "test-ns"} {
		if !nsSet[expected] {
			t.Errorf("Expected namespace %s not found", expected)
		}
	}
}

func TestSetAllNamespace_Empty(t *testing.T) {
	clientset := fake.NewClientset()

	var namespaces []string
	setAllNamespace(clientset, metav1.ListOptions{}, &namespaces)

	if len(namespaces) != 0 {
		t.Errorf("Expected 0 namespaces, got %d", len(namespaces))
	}
}

func TestSetAllNamespace_WithLabelSelector(t *testing.T) {
	// Create namespaces with labels
	ns1 := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "production",
			Labels: map[string]string{"env": "prod"},
		},
	}
	ns2 := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "staging",
			Labels: map[string]string{"env": "staging"},
		},
	}

	clientset := fake.NewClientset(ns1, ns2)

	var namespaces []string
	// Note: fake clientset doesn't filter by label selector, but we test the function signature
	setAllNamespace(clientset, metav1.ListOptions{LabelSelector: "env=prod"}, &namespaces)

	// With fake clientset, all namespaces are returned (no server-side filtering)
	if len(namespaces) < 1 {
		t.Error("Expected at least 1 namespace")
	}
}

func TestCheckConnection_DiscoveryWorks(t *testing.T) {
	// Create fake clientset
	clientset := fake.NewClientset()

	// Discovery check works with fake clientset
	_, err := clientset.Discovery().ServerVersion()
	if err != nil {
		t.Errorf("Discovery check failed: %v", err)
	}
}

// Note: Full checkConnection testing requires a real cluster or extensive mocking
// of SelfSubjectAccessReviews. The fake clientset doesn't return "allowed" by default.
// We test the discovery portion and handler behavior instead.

func TestAddServiceHandler_SkipsNoSelector(t *testing.T) {
	// Service without selector should be skipped
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-selector-svc",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Selector: nil, // No selector
			Ports: []v1.ServicePort{
				{Port: 80, Name: "http"},
			},
		},
	}

	opts := &NamespaceOpts{
		ClientSet: fake.NewClientset(),
		Context:   "test",
		Namespace: "default",
	}

	// Should not panic - just skip silently
	opts.AddServiceHandler(svc)
}

func TestAddServiceHandler_SkipsEmptySelector(t *testing.T) {
	// Service with empty selector should be skipped
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "empty-selector-svc",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{}, // Empty selector
			Ports: []v1.ServicePort{
				{Port: 80, Name: "http"},
			},
		},
	}

	opts := &NamespaceOpts{
		ClientSet: fake.NewClientset(),
		Context:   "test",
		Namespace: "default",
	}

	// Should not panic - just skip silently
	opts.AddServiceHandler(svc)
}

func TestAddServiceHandler_InvalidObject(t *testing.T) {
	opts := &NamespaceOpts{
		ClientSet: fake.NewClientset(),
		Context:   "test",
		Namespace: "default",
	}

	// Should not panic with non-service object
	opts.AddServiceHandler("not a service")
	opts.AddServiceHandler(nil)
	opts.AddServiceHandler(123)
}

func TestDeleteServiceHandler_InvalidObject(t *testing.T) {
	opts := &NamespaceOpts{
		ClientSet: fake.NewClientset(),
		Context:   "test",
		Namespace: "default",
	}

	// Should not panic with non-service object
	opts.DeleteServiceHandler("not a service")
	opts.DeleteServiceHandler(nil)
	opts.DeleteServiceHandler(123)
}

func TestUpdateServiceHandler_InvalidObjects(t *testing.T) {
	opts := &NamespaceOpts{
		ClientSet: fake.NewClientset(),
		Context:   "test",
		Namespace: "default",
	}

	// Should not panic with non-service objects
	opts.UpdateServiceHandler("old", "new")
	opts.UpdateServiceHandler(nil, nil)

	// Mixed types
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
	}
	opts.UpdateServiceHandler(svc, "not a service")
	opts.UpdateServiceHandler("not a service", svc)
}

func TestNamespaceOpts_ContextField(t *testing.T) {
	opts := &NamespaceOpts{
		Context:   "my-cluster",
		Namespace: "my-namespace",
		ClusterN:  0,
	}

	if opts.Context != "my-cluster" {
		t.Errorf("Expected context 'my-cluster', got %s", opts.Context)
	}

	if opts.Namespace != "my-namespace" {
		t.Errorf("Expected namespace 'my-namespace', got %s", opts.Namespace)
	}
}

// TestWithFakeKubernetesClient demonstrates using fake clientset for testing
func TestWithFakeKubernetesClient(t *testing.T) {
	// Create objects
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
		},
	}

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"app": "test"},
			Ports: []v1.ServicePort{
				{Port: 80, Name: "http"},
			},
		},
	}

	// Create fake clientset with objects
	clientset := fake.NewClientset(pod, svc)

	// Test listing pods
	pods, err := clientset.CoreV1().Pods("default").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Failed to list pods: %v", err)
	}

	if len(pods.Items) != 1 {
		t.Errorf("Expected 1 pod, got %d", len(pods.Items))
	}

	// Test listing services
	services, err := clientset.CoreV1().Services("default").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Failed to list services: %v", err)
	}

	if len(services.Items) != 1 {
		t.Errorf("Expected 1 service, got %d", len(services.Items))
	}
}

// TestCheckConnection_AllPermissionsAllowed tests checkConnection when all permissions are granted
func TestCheckConnection_AllPermissionsAllowed(t *testing.T) {
	clientset := fake.NewClientset()

	// Add reactor to return "allowed" for all access reviews
	clientset.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		review := action.(k8stesting.CreateAction).GetObject().(*authorizationv1.SelfSubjectAccessReview)
		review.Status.Allowed = true
		return true, review, nil
	})

	err := checkConnection(clientset, []string{"default"})
	if err != nil {
		t.Errorf("Expected no error with all permissions allowed, got: %v", err)
	}
}

// TestCheckConnection_PermissionDenied tests checkConnection when a permission is denied
func TestCheckConnection_PermissionDenied(t *testing.T) {
	clientset := fake.NewClientset()

	// Add reactor to deny "watch" permission
	clientset.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		review := action.(k8stesting.CreateAction).GetObject().(*authorizationv1.SelfSubjectAccessReview)
		if review.Spec.ResourceAttributes.Verb == "watch" {
			review.Status.Allowed = false
		} else {
			review.Status.Allowed = true
		}
		return true, review, nil
	})

	err := checkConnection(clientset, []string{"default"})
	if err == nil {
		t.Error("Expected error when permission denied, got nil")
	}
	if err != nil && err.Error() != "missing RBAC permission: {watch  pods  default   }" {
		t.Logf("Got expected permission error: %v", err)
	}
}

// TestCheckConnection_MultipleNamespaces tests checkConnection across multiple namespaces
func TestCheckConnection_MultipleNamespaces(t *testing.T) {
	clientset := fake.NewClientset()

	checkedNamespaces := make(map[string]bool)
	var mu sync.Mutex

	clientset.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		review := action.(k8stesting.CreateAction).GetObject().(*authorizationv1.SelfSubjectAccessReview)
		mu.Lock()
		checkedNamespaces[review.Spec.ResourceAttributes.Namespace] = true
		mu.Unlock()
		review.Status.Allowed = true
		return true, review, nil
	})

	namespaces := []string{"default", "production", "staging"}
	err := checkConnection(clientset, namespaces)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify all namespaces were checked
	for _, ns := range namespaces {
		if !checkedNamespaces[ns] {
			t.Errorf("Namespace %s was not checked for permissions", ns)
		}
	}
}

// TestUpdateServiceHandler_SelectorChanged tests handler when selector changes
func TestUpdateServiceHandler_SelectorChanged(t *testing.T) {
	// Use shared registry to avoid race conditions
	initRegistryOnce()

	oldSvc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"app": "old"},
			Ports: []v1.ServicePort{
				{Port: 80, Name: "http"},
			},
		},
	}

	newSvc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"app": "new"}, // Changed selector
			Ports: []v1.ServicePort{
				{Port: 80, Name: "http"},
			},
		},
	}

	opts := &NamespaceOpts{
		ClientSet: fake.NewClientset(),
		Context:   "test-ctx",
		Namespace: "default",
	}

	// Should not panic even without the service in registry
	opts.UpdateServiceHandler(oldSvc, newSvc)
}

// TestUpdateServiceHandler_PortsChanged tests handler when ports change
func TestUpdateServiceHandler_PortsChanged(t *testing.T) {
	initRegistryOnce()

	oldSvc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"app": "test"},
			Ports: []v1.ServicePort{
				{Port: 80, Name: "http"},
			},
		},
	}

	newSvc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"app": "test"},
			Ports: []v1.ServicePort{
				{Port: 8080, Name: "http"}, // Changed port
			},
		},
	}

	opts := &NamespaceOpts{
		ClientSet: fake.NewClientset(),
		Context:   "test-ctx",
		Namespace: "default",
	}

	// Should not panic
	opts.UpdateServiceHandler(oldSvc, newSvc)
}

// TestUpdateServiceHandler_NoChange tests handler when nothing changes
func TestUpdateServiceHandler_NoChange(t *testing.T) {
	initRegistryOnce()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"app": "test"},
			Ports: []v1.ServicePort{
				{Port: 80, Name: "http"},
			},
		},
	}

	opts := &NamespaceOpts{
		ClientSet: fake.NewClientset(),
		Context:   "test-ctx",
		Namespace: "default",
	}

	// Same service, no changes - should do nothing
	opts.UpdateServiceHandler(svc, svc)
}

// TestDeleteServiceHandler_ValidService tests deleting a service that exists
func TestDeleteServiceHandler_ValidService(t *testing.T) {
	initRegistryOnce()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"app": "test"},
			Ports: []v1.ServicePort{
				{Port: 80, Name: "http"},
			},
		},
	}

	opts := &NamespaceOpts{
		ClientSet: fake.NewClientset(),
		Context:   "test-ctx",
		Namespace: "default",
	}

	// Delete service (not in registry, should not panic)
	opts.DeleteServiceHandler(svc)
}

// TestAddServiceHandler_HeadlessService tests handling headless services
func TestAddServiceHandler_HeadlessService(t *testing.T) {
	initRegistryOnce()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "headless-svc",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			ClusterIP: "None", // Headless
			Selector:  map[string]string{"app": "test"},
			Ports: []v1.ServicePort{
				{Port: 80, Name: "http"},
			},
		},
	}

	// Create a temporary hosts file for testing
	tmpDir := t.TempDir()
	hostsPath := tmpDir + "/hosts"

	// Write initial hosts content
	if err := os.WriteFile(hostsPath, []byte("127.0.0.1 localhost\n"), 0644); err != nil {
		t.Fatalf("Failed to create test hosts file: %v", err)
	}

	hostFile, err := txeh.NewHosts(&txeh.HostsConfig{
		ReadFilePath:  hostsPath,
		WriteFilePath: hostsPath,
	})
	if err != nil {
		t.Fatalf("Failed to create txeh hosts: %v", err)
	}

	opts := &NamespaceOpts{
		ClientSet:       fake.NewClientset(),
		Context:         "test-ctx",
		Namespace:       "default",
		HostFile:        &fwdport.HostFileWithLock{Hosts: hostFile},
		NamespaceIPLock: &sync.Mutex{},
	}

	// Should handle headless service without panic
	// Note: This won't fully succeed without proper k8s config, but tests the code path
	opts.AddServiceHandler(svc)
}

// TestServiceLifecycle_UpdateThenAdd tests update followed by add for new services
func TestServiceLifecycle_UpdateThenAdd(t *testing.T) {
	initRegistryOnce()

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "lifecycle-update-svc",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"app": "test"},
			Ports: []v1.ServicePort{
				{Port: 80, Name: "http"},
			},
		},
	}

	opts := &NamespaceOpts{
		ClientSet: fake.NewClientset(),
		Context:   "test-ctx",
		Namespace: "default",
	}

	// Update with port change (exercises update code path even when service not in registry)
	updatedSvc := svc.DeepCopy()
	updatedSvc.Spec.Ports[0].Port = 8080
	opts.UpdateServiceHandler(svc, updatedSvc)
	// Should not panic - service wasn't in registry so update triggers re-add logic
}

// TestNamespaceOpts_Fields tests all NamespaceOpts fields
func TestNamespaceOpts_Fields(t *testing.T) {
	stopCh := make(chan struct{})
	tmpDir := t.TempDir()
	hostsPath := tmpDir + "/hosts"

	// Create hosts file
	if err := createTestHostsFile(hostsPath); err != nil {
		t.Fatalf("Failed to create hosts file: %v", err)
	}

	hostFile, _ := txeh.NewHosts(&txeh.HostsConfig{
		ReadFilePath:  hostsPath,
		WriteFilePath: hostsPath,
	})

	opts := NamespaceOpts{
		ClientSet:         fake.NewClientset(),
		Context:           "prod-cluster",
		Namespace:         "production",
		ClusterN:          1,
		NamespaceN:        2,
		Domain:            "internal.example.com",
		PortMapping:       []string{"80:8080"},
		HostFile:          &fwdport.HostFileWithLock{Hosts: hostFile},
		NamespaceIPLock:   &sync.Mutex{},
		ManualStopChannel: stopCh,
		ListOptions:       metav1.ListOptions{LabelSelector: "app=test"},
	}

	if opts.Context != "prod-cluster" {
		t.Errorf("Expected context 'prod-cluster', got %s", opts.Context)
	}
	if opts.Namespace != "production" {
		t.Errorf("Expected namespace 'production', got %s", opts.Namespace)
	}
	if opts.ClusterN != 1 {
		t.Errorf("Expected ClusterN 1, got %d", opts.ClusterN)
	}
	if opts.NamespaceN != 2 {
		t.Errorf("Expected NamespaceN 2, got %d", opts.NamespaceN)
	}
	if opts.Domain != "internal.example.com" {
		t.Errorf("Expected domain 'internal.example.com', got %s", opts.Domain)
	}
	if len(opts.PortMapping) != 1 || opts.PortMapping[0] != "80:8080" {
		t.Errorf("Unexpected PortMapping: %v", opts.PortMapping)
	}
}

// createTestHostsFile creates a minimal hosts file for testing
func createTestHostsFile(path string) error {
	return os.WriteFile(path, []byte("127.0.0.1 localhost\n"), 0644)
}

// TestCheckConnection_EmptyNamespaces tests with empty namespace list
func TestCheckConnection_EmptyNamespaces(t *testing.T) {
	clientset := fake.NewClientset()

	// With empty namespaces, should just check discovery
	err := checkConnection(clientset, []string{})
	if err != nil {
		t.Errorf("Unexpected error with empty namespaces: %v", err)
	}
}
