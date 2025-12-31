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
// URL structure:
//   - /           - Root (future web UI, currently redirects to /docs)
//   - /docs       - API documentation (Redoc)
//   - /api/...    - REST API endpoints
func (m *Manager) setupRouter() *gin.Engine {
	r := gin.New()

	// Global middleware
	r.Use(middleware.Recovery())
	r.Use(middleware.RequestLogger())
	r.Use(middleware.CORS())
	r.Use(middleware.NoCache())
	r.Use(middleware.ErrorHandler())

	// Root - placeholder for future web UI, redirects to docs for now
	r.GET("/", func(c *gin.Context) {
		c.Redirect(302, "/docs")
	})

	// Documentation endpoints
	docsHandler := handlers.NewDocsHandler(m.version)
	r.GET("/docs", docsHandler.Docs)
	r.GET("/docs/", docsHandler.Docs)
	r.GET("/openapi.yaml", docsHandler.OpenAPISpec)

	// API group
	api := r.Group("/api")
	{
		// Health endpoints
		healthHandler := handlers.NewHealthHandler(m.version, m.startTime, getManagerInfo)
		api.GET("/health", healthHandler.Health)
		api.GET("/info", healthHandler.Info)

		// API v1 routes
		v1 := api.Group("/v1")
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
			logsHandler := handlers.NewLogsHandler(m.stateReader, GetLogBufferProvider)
			v1.GET("/logs", logsHandler.Recent)
			v1.GET("/logs/stream", logsHandler.Stream)
			v1.GET("/logs/system", logsHandler.System)
			v1.DELETE("/logs/system", logsHandler.ClearSystem)

			// Events endpoint (SSE)
			eventsHandler := handlers.NewEventsHandler(m.eventStreamer)
			v1.GET("/events", eventsHandler.Stream)

			// Diagnostics endpoints
			diagnosticsHandler := handlers.NewDiagnosticsHandler(m.diagnosticsProvider)
			v1.GET("/diagnostics", diagnosticsHandler.Summary)
			v1.GET("/diagnostics/services/:key", diagnosticsHandler.ServiceDiagnostic)
			v1.GET("/diagnostics/forwards/:key", diagnosticsHandler.ForwardDiagnostic)
			v1.GET("/diagnostics/network", diagnosticsHandler.Network)
			v1.GET("/diagnostics/errors", diagnosticsHandler.Errors)

			// HTTP Traffic endpoints
			httpTrafficHandler := handlers.NewHTTPTrafficHandler(m.metricsProvider, m.stateReader)
			v1.GET("/forwards/:key/http", httpTrafficHandler.ForwardHTTP)
			v1.GET("/services/:key/http", httpTrafficHandler.ServiceHTTP)

			// AI-optimized analysis endpoints
			analyzeHandler := handlers.NewAnalyzeHandler(m.stateReader, m.diagnosticsProvider, getManagerInfo)
			v1.GET("/status", analyzeHandler.Status)
			v1.GET("/analyze", analyzeHandler.Analyze)

			// History endpoints
			historyHandler := handlers.NewHistoryHandler()
			v1.GET("/history/events", historyHandler.Events)
			v1.GET("/history/errors", historyHandler.Errors)
			v1.GET("/history/reconnections", historyHandler.AllReconnections)
			v1.GET("/history/stats", historyHandler.Stats)
			v1.GET("/services/:key/history/reconnections", historyHandler.Reconnections)

			// Namespace CRUD endpoints
			nsHandler := handlers.NewNamespacesHandler(m.namespaceController)
			v1.GET("/namespaces", nsHandler.List)
			v1.GET("/namespaces/:key", nsHandler.Get)
			v1.POST("/namespaces", nsHandler.Add)
			v1.DELETE("/namespaces/:key", nsHandler.Remove)

			// Service CRUD endpoints (add/remove)
			svcCRUDHandler := handlers.NewServicesCRUDHandler(m.serviceCRUD)
			v1.POST("/services", svcCRUDHandler.Add)
			v1.DELETE("/services/:key", svcCRUDHandler.Remove)

			// Kubernetes discovery endpoints
			k8sHandler := handlers.NewKubernetesHandler(m.k8sDiscovery)
			v1.GET("/kubernetes/namespaces", k8sHandler.ListNamespaces)
			v1.GET("/kubernetes/services", k8sHandler.ListServices)
			v1.GET("/kubernetes/services/:namespace/:name", k8sHandler.GetService)
			v1.GET("/kubernetes/contexts", k8sHandler.ListContexts)
			v1.GET("/kubernetes/pods/:namespace", k8sHandler.ListPods)
			v1.GET("/kubernetes/pods/:namespace/:podName", k8sHandler.GetPod)
			v1.GET("/kubernetes/pods/:namespace/:podName/logs", k8sHandler.GetPodLogs)
			v1.GET("/kubernetes/events/:namespace", k8sHandler.GetEvents)
			v1.GET("/kubernetes/endpoints/:namespace/:serviceName", k8sHandler.GetEndpoints)
		}
	}

	return r
}
