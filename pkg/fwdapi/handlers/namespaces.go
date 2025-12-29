package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
)

// NamespacesHandler handles namespace CRUD operations
type NamespacesHandler struct {
	controller types.NamespaceController
}

// NewNamespacesHandler creates a new namespaces handler
func NewNamespacesHandler(controller types.NamespaceController) *NamespacesHandler {
	return &NamespacesHandler{
		controller: controller,
	}
}

// List returns all watched namespaces
// GET /v1/namespaces
func (h *NamespacesHandler) List(c *gin.Context) {
	if h.controller == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "namespace_controller_unavailable",
				Message: "Namespace controller not configured",
			},
		})
		return
	}

	namespaces := h.controller.ListNamespaces()

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data: types.NamespaceListResponse{
			Namespaces: namespaces,
		},
		Meta: &types.MetaInfo{
			Count:     len(namespaces),
			Timestamp: time.Now(),
		},
	})
}

// Get returns details for a specific watched namespace
// GET /v1/namespaces/:key
func (h *NamespacesHandler) Get(c *gin.Context) {
	if h.controller == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "namespace_controller_unavailable",
				Message: "Namespace controller not configured",
			},
		})
		return
	}

	key := c.Param("key")

	// Parse key as namespace.context
	namespace, ctx := parseNamespaceKey(key)
	if namespace == "" {
		c.JSON(http.StatusBadRequest, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "invalid_key",
				Message: "Key must be in format 'namespace.context'",
			},
		})
		return
	}

	info, err := h.controller.GetNamespace(ctx, namespace)
	if err != nil {
		c.JSON(http.StatusNotFound, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "namespace_not_found",
				Message: err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data:    info,
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}

// Add starts watching a namespace
// POST /v1/namespaces
func (h *NamespacesHandler) Add(c *gin.Context) {
	if h.controller == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "namespace_controller_unavailable",
				Message: "Namespace controller not configured",
			},
		})
		return
	}

	var req types.AddNamespaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "invalid_request",
				Message: err.Error(),
			},
		})
		return
	}

	// Use provided context (empty means current context will be used)
	ctx := req.Context

	opts := types.AddNamespaceOpts{
		LabelSelector: req.Selector,
	}

	info, err := h.controller.AddNamespace(ctx, req.Namespace, opts)
	if err != nil {
		c.JSON(http.StatusConflict, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "add_namespace_failed",
				Message: err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusCreated, types.Response{
		Success: true,
		Data: types.AddNamespaceResponse{
			Key:       info.Key,
			Namespace: info.Namespace,
			Context:   info.Context,
			Services:  []string{}, // Services will be discovered asynchronously
		},
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}

// Remove stops watching a namespace
// DELETE /v1/namespaces/:key
func (h *NamespacesHandler) Remove(c *gin.Context) {
	if h.controller == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "namespace_controller_unavailable",
				Message: "Namespace controller not configured",
			},
		})
		return
	}

	key := c.Param("key")

	// Parse key as namespace.context
	namespace, ctx := parseNamespaceKey(key)
	if namespace == "" {
		c.JSON(http.StatusBadRequest, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "invalid_key",
				Message: "Key must be in format 'namespace.context'",
			},
		})
		return
	}

	err := h.controller.RemoveNamespace(ctx, namespace)
	if err != nil {
		c.JSON(http.StatusNotFound, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "remove_namespace_failed",
				Message: err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, types.Response{
		Success: true,
		Data: types.RemoveResponse{
			Removed: true,
			Key:     key,
			Message: "Namespace watcher stopped and services removed",
		},
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}

// parseNamespaceKey parses "namespace.context" into namespace and context
func parseNamespaceKey(key string) (namespace, context string) {
	// Find the first dot
	for i := 0; i < len(key); i++ {
		if key[i] == '.' {
			return key[:i], key[i+1:]
		}
	}
	return "", ""
}
