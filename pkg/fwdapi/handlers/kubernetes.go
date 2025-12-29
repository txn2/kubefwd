package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
)

// KubernetesHandler handles Kubernetes resource discovery
type KubernetesHandler struct {
	discovery types.KubernetesDiscovery
}

// NewKubernetesHandler creates a new kubernetes discovery handler
func NewKubernetesHandler(discovery types.KubernetesDiscovery) *KubernetesHandler {
	return &KubernetesHandler{
		discovery: discovery,
	}
}

// ListNamespaces returns available namespaces in the cluster
// GET /v1/kubernetes/namespaces
func (h *KubernetesHandler) ListNamespaces(c *gin.Context) {
	if h.discovery == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "kubernetes_discovery_unavailable",
				Message: "Kubernetes discovery not configured",
			},
		})
		return
	}

	// Get context from query param (empty means current context)
	ctx := c.Query("context")

	namespaces, err := h.discovery.ListNamespaces(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "list_namespaces_failed",
				Message: err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data: types.K8sNamespacesResponse{
			Namespaces: namespaces,
		},
		Meta: &types.MetaInfo{
			Count:     len(namespaces),
			Timestamp: time.Now(),
		},
	})
}

// ListServices returns available services in a namespace
// GET /v1/kubernetes/services
func (h *KubernetesHandler) ListServices(c *gin.Context) {
	if h.discovery == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "kubernetes_discovery_unavailable",
				Message: "Kubernetes discovery not configured",
			},
		})
		return
	}

	namespace := c.Query("namespace")
	if namespace == "" {
		c.JSON(http.StatusBadRequest, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "missing_namespace",
				Message: "namespace query parameter is required",
			},
		})
		return
	}

	ctx := c.Query("context")

	services, err := h.discovery.ListServices(ctx, namespace)
	if err != nil {
		c.JSON(http.StatusInternalServerError, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "list_services_failed",
				Message: err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data: types.K8sServicesResponse{
			Services: services,
		},
		Meta: &types.MetaInfo{
			Count:     len(services),
			Timestamp: time.Now(),
		},
	})
}

// GetService returns details for a specific service
// GET /v1/kubernetes/services/:namespace/:name
func (h *KubernetesHandler) GetService(c *gin.Context) {
	if h.discovery == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "kubernetes_discovery_unavailable",
				Message: "Kubernetes discovery not configured",
			},
		})
		return
	}

	namespace := c.Param("namespace")
	name := c.Param("name")

	if namespace == "" || name == "" {
		c.JSON(http.StatusBadRequest, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "missing_params",
				Message: "namespace and name are required",
			},
		})
		return
	}

	ctx := c.Query("context")
	if ctx == "" {
		ctx = "default"
	}

	service, err := h.discovery.GetService(ctx, namespace, name)
	if err != nil {
		c.JSON(http.StatusNotFound, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "service_not_found",
				Message: err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    service,
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}

// ListContexts returns available Kubernetes contexts
// GET /v1/kubernetes/contexts
func (h *KubernetesHandler) ListContexts(c *gin.Context) {
	if h.discovery == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "kubernetes_discovery_unavailable",
				Message: "Kubernetes discovery not configured",
			},
		})
		return
	}

	contexts, err := h.discovery.ListContexts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "list_contexts_failed",
				Message: err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    contexts,
		Meta: &types.MetaInfo{
			Count:     len(contexts.Contexts),
			Timestamp: time.Now(),
		},
	})
}
