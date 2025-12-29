package handlers

import (
	_ "embed"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed openapi.yaml
var openapiSpec []byte

// DocsHandler handles API documentation endpoints
type DocsHandler struct {
	version string
}

// NewDocsHandler creates a new DocsHandler with the kubefwd version
func NewDocsHandler(version string) *DocsHandler {
	return &DocsHandler{
		version: version,
	}
}

// redocHTML is the HTML template for Redoc documentation
const redocHTML = `<!DOCTYPE html>
<html>
<head>
    <title>kubefwd API Documentation</title>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <link href="https://fonts.googleapis.com/css?family=Montserrat:300,400,700|Roboto:300,400,700" rel="stylesheet">
    <style>
        body {
            margin: 0;
            padding: 0;
        }
    </style>
</head>
<body>
    <redoc spec-url='/openapi.yaml'
           hide-download-button="false"
           theme='{
               "colors": {
                   "primary": {
                       "main": "#3b82f6"
                   }
               },
               "typography": {
                   "fontSize": "15px",
                   "fontFamily": "Roboto, sans-serif",
                   "headings": {
                       "fontFamily": "Montserrat, sans-serif"
                   }
               },
               "sidebar": {
                   "width": "260px"
               }
           }'
    ></redoc>
    <script src="https://cdn.redoc.ly/redoc/latest/bundles/redoc.standalone.js"></script>
</body>
</html>`

// Docs serves the Redoc documentation HTML page
func (h *DocsHandler) Docs(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, redocHTML)
}

// OpenAPISpec serves the OpenAPI specification YAML file with the actual version
func (h *DocsHandler) OpenAPISpec(c *gin.Context) {
	// Replace the placeholder version with the actual kubefwd version
	spec := string(openapiSpec)
	spec = strings.Replace(spec, `version: "0.0.0"`, `version: "`+h.version+`"`, 1)

	c.Header("Content-Type", "application/x-yaml")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Data(http.StatusOK, "application/x-yaml", []byte(spec))
}
