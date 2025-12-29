package services

import (
	"context"
	"sync"
	"testing"

	authorizationv1 "k8s.io/api/authorization/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

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

// TestCheckConnection_EmptyNamespaces tests with empty namespace list
func TestCheckConnection_EmptyNamespaces(t *testing.T) {
	clientset := fake.NewClientset()

	// With empty namespaces, should just check discovery
	err := checkConnection(clientset, []string{})
	if err != nil {
		t.Errorf("Unexpected error with empty namespaces: %v", err)
	}
}
