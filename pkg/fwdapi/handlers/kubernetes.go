package handlers

import (
	"net/http"
	"strconv"
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

// GetPodLogs returns logs from a pod
// GET /v1/kubernetes/pods/:namespace/:podName/logs
func (h *KubernetesHandler) GetPodLogs(c *gin.Context) {
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
	podName := c.Param("podName")

	if namespace == "" || podName == "" {
		c.JSON(http.StatusBadRequest, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "missing_params",
				Message: "namespace and podName are required",
			},
		})
		return
	}

	ctx := c.Query("context")

	opts := types.PodLogsOptions{
		Container:  c.Query("container"),
		SinceTime:  c.Query("since_time"),
		Previous:   c.Query("previous") == "true",
		Timestamps: c.Query("timestamps") == "true",
	}

	if tailLines := c.Query("tail_lines"); tailLines != "" {
		if n, err := strconv.Atoi(tailLines); err == nil {
			opts.TailLines = n
		}
	}

	logs, err := h.discovery.GetPodLogs(ctx, namespace, podName, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "get_pod_logs_failed",
				Message: err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    logs,
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}

// ListPods returns pods in a namespace
// GET /v1/kubernetes/pods/:namespace
func (h *KubernetesHandler) ListPods(c *gin.Context) {
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
	if namespace == "" {
		c.JSON(http.StatusBadRequest, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "missing_namespace",
				Message: "namespace is required",
			},
		})
		return
	}

	ctx := c.Query("context")

	opts := types.ListPodsOptions{
		LabelSelector: c.Query("label_selector"),
		FieldSelector: c.Query("field_selector"),
		ServiceName:   c.Query("service_name"),
	}

	pods, err := h.discovery.ListPods(ctx, namespace, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "list_pods_failed",
				Message: err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    pods,
		Meta: &types.MetaInfo{
			Count:     len(pods),
			Timestamp: time.Now(),
		},
	})
}

// GetPod returns details for a specific pod
// GET /v1/kubernetes/pods/:namespace/:podName
func (h *KubernetesHandler) GetPod(c *gin.Context) {
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
	podName := c.Param("podName")

	if namespace == "" || podName == "" {
		c.JSON(http.StatusBadRequest, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "missing_params",
				Message: "namespace and podName are required",
			},
		})
		return
	}

	ctx := c.Query("context")

	pod, err := h.discovery.GetPod(ctx, namespace, podName)
	if err != nil {
		c.JSON(http.StatusNotFound, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "pod_not_found",
				Message: err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    pod,
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}

// GetEvents returns Kubernetes events for a namespace
// GET /v1/kubernetes/events/:namespace
func (h *KubernetesHandler) GetEvents(c *gin.Context) {
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
	if namespace == "" {
		c.JSON(http.StatusBadRequest, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "missing_namespace",
				Message: "namespace is required",
			},
		})
		return
	}

	ctx := c.Query("context")

	opts := types.GetEventsOptions{
		ResourceKind: c.Query("resource_kind"),
		ResourceName: c.Query("resource_name"),
	}

	if limit := c.Query("limit"); limit != "" {
		if n, err := strconv.Atoi(limit); err == nil {
			opts.Limit = n
		}
	}

	events, err := h.discovery.GetEvents(ctx, namespace, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "get_events_failed",
				Message: err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    events,
		Meta: &types.MetaInfo{
			Count:     len(events),
			Timestamp: time.Now(),
		},
	})
}

// GetEndpoints returns endpoints for a service
// GET /v1/kubernetes/endpoints/:namespace/:serviceName
func (h *KubernetesHandler) GetEndpoints(c *gin.Context) {
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
	serviceName := c.Param("serviceName")

	if namespace == "" || serviceName == "" {
		c.JSON(http.StatusBadRequest, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "missing_params",
				Message: "namespace and serviceName are required",
			},
		})
		return
	}

	ctx := c.Query("context")

	endpoints, err := h.discovery.GetEndpoints(ctx, namespace, serviceName)
	if err != nil {
		c.JSON(http.StatusNotFound, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "endpoints_not_found",
				Message: err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    endpoints,
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}
