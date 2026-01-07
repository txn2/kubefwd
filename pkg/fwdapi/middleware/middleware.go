package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// Recovery middleware with logging
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("API panic recovered: %v", err)
				c.JSON(500, gin.H{
					"success": false,
					"error": gin.H{
						"code":    "INTERNAL_ERROR",
						"message": "Internal server error",
					},
				})
				c.Abort()
			}
		}()
		c.Next()
	}
}

// RequestLogger logs API requests at debug level
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		if query != "" {
			path = path + "?" + query
		}

		log.Debugf("API %s %s %d %v", c.Request.Method, path, status, latency)
	}
}

// allowedOrigins lists origins permitted to make cross-origin requests.
// Restricted to local origins for security since kubefwd runs with root privileges.
var allowedOrigins = map[string]bool{
	"http://kubefwd.internal": true,
	"http://localhost":        true,
	"http://localhost:8080":   true,
	"http://127.0.0.1":        true,
	"http://127.0.0.1:8080":   true,
	"http://127.2.27.1":       true, // kubefwd API IP
	"http://127.2.27.1:8080":  true,
}

// CORS middleware for browser access.
// Restricts cross-origin requests to trusted local origins only.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Allow requests with no Origin header (same-origin, curl, etc.)
		// or from explicitly allowed origins
		if origin == "" || allowedOrigins[origin] {
			if origin != "" {
				c.Header("Access-Control-Allow-Origin", origin)
			}
			c.Header("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept")
			c.Header("Access-Control-Max-Age", "86400")
			c.Header("Vary", "Origin")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// ErrorHandler standardizes error responses
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) > 0 {
			err := c.Errors.Last()
			status := c.Writer.Status()
			if status == 200 {
				status = 500
			}
			c.JSON(status, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "REQUEST_ERROR",
					"message": err.Error(),
				},
			})
		}
	}
}

// NoCache middleware prevents caching of API responses
func NoCache() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.Next()
	}
}
