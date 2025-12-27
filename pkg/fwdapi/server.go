package fwdapi

import (
	"github.com/gin-gonic/gin"
	"github.com/txn2/kubefwd/pkg/fwdapi/handlers"
	"github.com/txn2/kubefwd/pkg/fwdapi/middleware"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
)

// getManagerInfo returns a function that provides ManagerInfo interface
func getManagerInfo() types.ManagerInfo {
	if mgr := GetManager(); mgr != nil {
		return mgr
	}
	return nil
}

// setupRouter creates and configures the Gin router with all routes
func (m *Manager) setupRouter() *gin.Engine {
	r := gin.New()

	// Global middleware
	r.Use(middleware.Recovery())
	r.Use(middleware.RequestLogger())
	r.Use(middleware.CORS())
	r.Use(middleware.NoCache())
	r.Use(middleware.ErrorHandler())

	// Health endpoints (unversioned)
	healthHandler := handlers.NewHealthHandler(m.version, m.startTime, getManagerInfo)
	r.GET("/", healthHandler.Root)
	r.GET("/health", healthHandler.Health)
	r.GET("/info", healthHandler.Info)

	// API v1 routes
	v1 := r.Group("/v1")
	{
		// Services endpoints
		svcHandler := handlers.NewServicesHandler(m.stateReader, m.serviceController)
		v1.GET("/services", svcHandler.List)
		v1.GET("/services/:key", svcHandler.Get)
		v1.POST("/services/:key/reconnect", svcHandler.Reconnect)
		v1.POST("/services/:key/sync", svcHandler.Sync)
		v1.POST("/services/reconnect", svcHandler.ReconnectAll)

		// Forwards endpoints
		fwdHandler := handlers.NewForwardsHandler(m.stateReader)
		v1.GET("/forwards", fwdHandler.List)
		v1.GET("/forwards/:key", fwdHandler.Get)

		// Metrics endpoints
		metricsHandler := handlers.NewMetricsHandler(m.stateReader, m.metricsProvider, getManagerInfo)
		v1.GET("/metrics", metricsHandler.Summary)
		v1.GET("/metrics/services", metricsHandler.ByService)
		v1.GET("/metrics/services/:key", metricsHandler.ServiceDetail)
		v1.GET("/metrics/services/:key/history", metricsHandler.ServiceHistory)

		// Logs endpoints
		logsHandler := handlers.NewLogsHandler(m.stateReader)
		v1.GET("/logs", logsHandler.Recent)
		v1.GET("/logs/stream", logsHandler.Stream)

		// Events endpoint (SSE)
		eventsHandler := handlers.NewEventsHandler(m.eventStreamer)
		v1.GET("/events", eventsHandler.Stream)
	}

	return r
}
