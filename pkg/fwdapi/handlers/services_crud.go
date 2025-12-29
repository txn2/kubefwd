package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
)

// ServicesCRUDHandler handles service add/remove operations
type ServicesCRUDHandler struct {
	serviceCRUD types.ServiceCRUD
}

// NewServicesCRUDHandler creates a new services CRUD handler
func NewServicesCRUDHandler(serviceCRUD types.ServiceCRUD) *ServicesCRUDHandler {
	return &ServicesCRUDHandler{
		serviceCRUD: serviceCRUD,
	}
}

// Add forwards a specific service
// POST /v1/services
func (h *ServicesCRUDHandler) Add(c *gin.Context) {
	if h.serviceCRUD == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "service_crud_unavailable",
				Message: "Service CRUD controller not configured",
			},
		})
		return
	}

	var req types.AddServiceRequest
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

	resp, err := h.serviceCRUD.AddService(req)
	if err != nil {
		c.JSON(http.StatusConflict, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "add_service_failed",
				Message: err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusCreated, types.Response{
		Success: true,
		Data:    resp,
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}

// Remove stops forwarding a service
// DELETE /v1/services/:key
func (h *ServicesCRUDHandler) Remove(c *gin.Context) {
	if h.serviceCRUD == nil {
		c.JSON(http.StatusServiceUnavailable, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "service_crud_unavailable",
				Message: "Service CRUD controller not configured",
			},
		})
		return
	}

	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "invalid_key",
				Message: "Service key is required",
			},
		})
		return
	}

	err := h.serviceCRUD.RemoveService(key)
	if err != nil {
		c.JSON(http.StatusNotFound, types.Response{
			Success: false,
			Error: &types.ErrorInfo{
				Code:    "remove_service_failed",
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
			Message: "Service forwarding stopped",
		},
		Meta: &types.MetaInfo{
			Timestamp: time.Now(),
		},
	})
}
