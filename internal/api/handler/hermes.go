package handler

import (
	"net/http/httputil"
	"net/url"

	"github.com/gin-gonic/gin"
)

// RegisterHermesProxy registers the Hermes reverse proxy on the router when
// HERMES_URL is configured. If hermesURL is empty, it is a no-op.
func RegisterHermesProxy(router *gin.Engine, hermesURL string) {
	if hermesURL == "" {
		return
	}
	router.Any("/api/v1/hermes/*path", hermesProxyHandler(hermesURL))
}

// hermesProxyHandler builds a single-host reverse proxy to the Hermes service.
func hermesProxyHandler(hermesURL string) gin.HandlerFunc {
	target, _ := url.Parse(hermesURL)
	p := httputil.NewSingleHostReverseProxy(target)
	return func(c *gin.Context) {
		c.Request.URL.Path = c.Param("path")
		p.ServeHTTP(c.Writer, c.Request)
	}
}
