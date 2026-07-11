// Package apidocs serves the self-hosted Scalar API reference and authoritative OpenAPI copy.
package apidocs

import (
	"embed"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

const docsHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>SCYG Blog API Reference</title></head>
<body><div id="app"></div><script id="api-reference" data-url="/openapi.yaml" data-configuration='{&quot;telemetry&quot;:false}' src="/docs/assets/scalar.js"></script></body></html>`

//go:generate go run ../../../docsync

//go:embed assets/openapi.yaml assets/scalar.js assets/LICENSE.scalar assets/manifest.json
var assets embed.FS

// Mount registers all documentation routes when enabled and none when disabled.
func Mount(router *gin.Engine, enabled bool) error {
	if !enabled {
		return nil
	}
	for _, route := range router.Routes() {
		if route.Path == "/docs" || route.Path == "/openapi.yaml" || route.Path == "/docs/assets/scalar.js" {
			return fmt.Errorf("API documentation route already mounted: %s", route.Path)
		}
	}
	router.GET("/docs", serveDocs)
	router.GET("/openapi.yaml", serveOpenAPI)
	router.GET("/docs/assets/scalar.js", serveScalar)
	return nil
}

func serveDocs(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	ctx.Data(http.StatusOK, "text/html; charset=utf-8", []byte(docsHTML))
}

func serveOpenAPI(ctx *gin.Context) {
	serveAsset(ctx, "assets/openapi.yaml", "application/yaml; charset=utf-8", "no-cache")
}

func serveScalar(ctx *gin.Context) {
	serveAsset(ctx, "assets/scalar.js", "text/javascript; charset=utf-8", "public, max-age=31536000, immutable")
}

func serveAsset(ctx *gin.Context, name, contentType, cacheControl string) {
	content, err := assets.ReadFile(name)
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	ctx.Header("Cache-Control", cacheControl)
	ctx.Data(http.StatusOK, contentType, content)
}
