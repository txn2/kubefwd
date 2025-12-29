// Package testutil provides test utilities for MCP integration testing.
package testutil

import (
	"time"

	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

// DemoServices contains test data mirroring test/manifests/demo-microservices.yaml
// kft1: 15 services (e-commerce platform)
// kft2: 15 services (devops platform)
// Each service has a regular and headless variant = 30 services per namespace

// KFT1Services returns test services for the kft1 (e-commerce) namespace.
func KFT1Services() []ServiceFixture {
	return []ServiceFixture{
		{Name: "api-gateway", Ports: []int{80}, Containers: 1},
		{Name: "frontend-web", Ports: []int{80}, Containers: 1},
		{Name: "user-api", Ports: []int{80, 6379}, Containers: 2},
		{Name: "auth-service", Ports: []int{80, 6379}, Containers: 2},
		{Name: "product-catalog", Ports: []int{80, 5432, 6379}, Containers: 3},
		{Name: "order-service", Ports: []int{80, 5432, 6379}, Containers: 3},
		{Name: "payment-gateway", Ports: []int{80, 5432}, Containers: 2},
		{Name: "inventory-api", Ports: []int{80, 6379}, Containers: 2},
		{Name: "notification-hub", Ports: []int{80, 6379, 11211, 8888}, Containers: 4},
		{Name: "search-engine", Ports: []int{80}, Containers: 1},
		{Name: "analytics-backend", Ports: []int{80, 5432, 6379}, Containers: 3},
		{Name: "cache-primary", Ports: []int{6379}, Containers: 1},
		{Name: "db-primary", Ports: []int{5432}, Containers: 1},
		{Name: "events-processor", Ports: []int{80, 6379, 6380, 11211}, Containers: 4},
		{Name: "service-mesh", Ports: []int{80, 8080, 6379, 11211, 9000}, Containers: 5},
	}
}

// KFT2Services returns test services for the kft2 (devops) namespace.
func KFT2Services() []ServiceFixture {
	return []ServiceFixture{
		{Name: "dashboard-ui", Ports: []int{80}, Containers: 1},
		{Name: "api-server", Ports: []int{80, 6379}, Containers: 2},
		{Name: "metrics-collector", Ports: []int{80, 6379, 9090}, Containers: 3},
		{Name: "log-aggregator", Ports: []int{80, 6379}, Containers: 2},
		{Name: "alert-manager", Ports: []int{80, 6379}, Containers: 2},
		{Name: "config-server", Ports: []int{80}, Containers: 1},
		{Name: "secret-vault", Ports: []int{80, 6379}, Containers: 2},
		{Name: "ci-runner", Ports: []int{80, 6379, 5432}, Containers: 3},
		{Name: "cd-deployer", Ports: []int{80, 6379}, Containers: 2},
		{Name: "registry-proxy", Ports: []int{80}, Containers: 1},
		{Name: "artifact-store", Ports: []int{80, 5432, 6379}, Containers: 3},
		{Name: "monitoring-stack", Ports: []int{80, 6379, 11211, 9100}, Containers: 4},
		{Name: "backup-service", Ports: []int{80, 5432}, Containers: 2},
		{Name: "audit-logger", Ports: []int{80, 5432, 6379}, Containers: 3},
		{Name: "gateway-mesh", Ports: []int{80, 8080, 6379, 11211, 9000}, Containers: 5},
	}
}

// ServiceFixture represents a test service configuration.
type ServiceFixture struct {
	Name       string
	Ports      []int
	Containers int
}

// GenerateServiceSnapshots generates ServiceSnapshot test data for a namespace.
func GenerateServiceSnapshots(namespace, context string, services []ServiceFixture, includeHeadless bool) []state.ServiceSnapshot {
	var result []state.ServiceSnapshot

	for _, svc := range services {
		// Regular service
		key := svc.Name + "." + namespace + "." + context
		result = append(result, state.ServiceSnapshot{
			Key:         key,
			ServiceName: svc.Name,
			Namespace:   namespace,
			Context:     context,
			Headless:    false,
			ActiveCount: 1,
			ErrorCount:  0,
		})

		// Headless service
		if includeHeadless {
			headlessKey := svc.Name + "-headless." + namespace + "." + context
			result = append(result, state.ServiceSnapshot{
				Key:         headlessKey,
				ServiceName: svc.Name + "-headless",
				Namespace:   namespace,
				Context:     context,
				Headless:    true,
				ActiveCount: 1,
				ErrorCount:  0,
			})
		}
	}

	return result
}

// GenerateForwardSnapshots generates ForwardSnapshot test data for a namespace.
func GenerateForwardSnapshots(namespace, context string, services []ServiceFixture, includeHeadless bool) []state.ForwardSnapshot {
	var result []state.ForwardSnapshot
	baseIP := 127*256*256*256 + 1*256*256 // 127.1.x.x
	ipOffset := 0

	for _, svc := range services {
		serviceKey := svc.Name + "." + namespace + "." + context
		podName := svc.Name + "-abc123-xyz"

		for _, port := range svc.Ports {
			ip := baseIP + ipOffset
			localIP := formatIP(ip)
			ipOffset++

			key := serviceKey + "." + podName + "." + itoa(port)
			result = append(result, state.ForwardSnapshot{
				Key:         key,
				ServiceKey:  serviceKey,
				ServiceName: svc.Name,
				Namespace:   namespace,
				Context:     context,
				PodName:     podName,
				LocalIP:     localIP,
				LocalPort:   itoa(port),
				PodPort:     itoa(port),
				Hostnames:   []string{svc.Name + "." + namespace},
				Status:      state.StatusActive,
				Headless:    false,
				StartedAt:   time.Now(),
				LastActive:  time.Now(),
			})
		}

		// Headless service forwards
		if includeHeadless {
			headlessServiceKey := svc.Name + "-headless." + namespace + "." + context
			headlessPodName := podName + ".headless"

			for _, port := range svc.Ports {
				ip := baseIP + ipOffset
				localIP := formatIP(ip)
				ipOffset++

				// Headless services have pod-specific service names
				podServiceName := headlessPodName + "." + svc.Name + "-headless"
				key := headlessServiceKey + "." + headlessPodName + "." + itoa(port)

				result = append(result, state.ForwardSnapshot{
					Key:         key,
					ServiceKey:  headlessServiceKey,
					ServiceName: podServiceName,
					Namespace:   namespace,
					Context:     context,
					PodName:     headlessPodName,
					LocalIP:     localIP,
					LocalPort:   itoa(port),
					PodPort:     itoa(port),
					Hostnames:   []string{podServiceName + "." + namespace},
					Status:      state.StatusActive,
					Headless:    true,
					StartedAt:   time.Now(),
					LastActive:  time.Now(),
				})
			}
		}
	}

	return result
}

// formatIP converts an integer to IP string format.
func formatIP(ip int) string {
	return itoa((ip>>24)&255) + "." + itoa((ip>>16)&255) + "." + itoa((ip>>8)&255) + "." + itoa(ip&255)
}

// itoa converts int to string without importing strconv.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	result := ""
	for i > 0 {
		result = string(rune('0'+i%10)) + result
		i /= 10
	}
	return result
}

// PopulateStore populates a state store with test data.
func PopulateStore(store *state.Store, namespace, context string, services []ServiceFixture, includeHeadless bool) {
	forwards := GenerateForwardSnapshots(namespace, context, services, includeHeadless)
	for _, fwd := range forwards {
		store.AddForward(fwd)
	}
}

// PopulateStoreKFT1 populates a store with kft1 test data.
func PopulateStoreKFT1(store *state.Store, context string) {
	PopulateStore(store, "kft1", context, KFT1Services(), true)
}

// PopulateStoreKFT2 populates a store with kft2 test data.
func PopulateStoreKFT2(store *state.Store, context string) {
	PopulateStore(store, "kft2", context, KFT2Services(), true)
}

// ContextFixture provides a standard test context name.
const ContextFixture = "test-context"
