package testutil

import (
	"testing"

	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

// AssertStoreEmpty verifies the store has no forwards or services.
func AssertStoreEmpty(t *testing.T, store *state.Store) {
	t.Helper()

	if store.Count() != 0 {
		t.Errorf("Expected store to be empty, got %d forwards", store.Count())
	}

	if store.ServiceCount() != 0 {
		t.Errorf("Expected no services, got %d", store.ServiceCount())
	}
}

// AssertStoreForwardCount verifies the store has the expected number of forwards.
func AssertStoreForwardCount(t *testing.T, store *state.Store, expected int) {
	t.Helper()

	if store.Count() != expected {
		t.Errorf("Expected %d forwards, got %d", expected, store.Count())
	}
}

// AssertStoreServiceCount verifies the store has the expected number of services.
func AssertStoreServiceCount(t *testing.T, store *state.Store, expected int) {
	t.Helper()

	if store.ServiceCount() != expected {
		t.Errorf("Expected %d services, got %d", expected, store.ServiceCount())
	}
}

// AssertNoForwardsForNamespace verifies no forwards exist for a namespace.
func AssertNoForwardsForNamespace(t *testing.T, store *state.Store, namespace, context string) {
	t.Helper()

	forwards := store.GetFiltered()
	for _, fwd := range forwards {
		if fwd.Namespace == namespace && fwd.Context == context {
			t.Errorf("Found unexpected forward for %s.%s: %s", namespace, context, fwd.Key)
		}
	}
}

// AssertNoServicesForNamespace verifies no services exist for a namespace.
func AssertNoServicesForNamespace(t *testing.T, store *state.Store, namespace, context string) {
	t.Helper()

	services := store.GetServices()
	for _, svc := range services {
		if svc.Namespace == namespace && svc.Context == context {
			t.Errorf("Found unexpected service for %s.%s: %s", namespace, context, svc.Key)
		}
	}
}

// AssertForwardsExistForNamespace verifies forwards exist for a namespace.
func AssertForwardsExistForNamespace(t *testing.T, store *state.Store, namespace, context string, minimum int) {
	t.Helper()

	count := 0
	forwards := store.GetFiltered()
	for _, fwd := range forwards {
		if fwd.Namespace == namespace && fwd.Context == context {
			count++
		}
	}

	if count < minimum {
		t.Errorf("Expected at least %d forwards for %s.%s, got %d", minimum, namespace, context, count)
	}
}

// AssertForwardExists verifies a specific forward exists.
func AssertForwardExists(t *testing.T, store *state.Store, key string) {
	t.Helper()

	if store.GetForward(key) == nil {
		t.Errorf("Expected forward %s to exist", key)
	}
}

// AssertForwardNotExists verifies a specific forward does NOT exist.
func AssertForwardNotExists(t *testing.T, store *state.Store, key string) {
	t.Helper()

	if store.GetForward(key) != nil {
		t.Errorf("Expected forward %s to NOT exist", key)
	}
}

// AssertServiceExists verifies a specific service exists.
func AssertServiceExists(t *testing.T, store *state.Store, key string) {
	t.Helper()

	if store.GetService(key) == nil {
		t.Errorf("Expected service %s to exist", key)
	}
}

// AssertServiceNotExists verifies a specific service does NOT exist.
func AssertServiceNotExists(t *testing.T, store *state.Store, key string) {
	t.Helper()

	if store.GetService(key) != nil {
		t.Errorf("Expected service %s to NOT exist", key)
	}
}

// CountForwardsForNamespace counts forwards for a namespace.
func CountForwardsForNamespace(store *state.Store, namespace, context string) int {
	count := 0
	forwards := store.GetFiltered()
	for _, fwd := range forwards {
		if fwd.Namespace == namespace && fwd.Context == context {
			count++
		}
	}
	return count
}

// CountServicesForNamespace counts services for a namespace.
func CountServicesForNamespace(store *state.Store, namespace, context string) int {
	count := 0
	services := store.GetServices()
	for _, svc := range services {
		if svc.Namespace == namespace && svc.Context == context {
			count++
		}
	}
	return count
}
