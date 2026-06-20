package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func CORS() gin.HandlerFunc {
	rawOrigins := os.Getenv("FRONTEND_URL")
	if rawOrigins == "" {
		rawOrigins = os.Getenv("CORS_ALLOWED_ORIGINS")
	}
	if rawOrigins == "" {
		rawOrigins = "http://localhost:3000"
	}

	allowedOrigins := make(map[string]struct{})
	for _, o := range strings.Split(rawOrigins, ",") {
		allowedOrigins[strings.TrimSpace(o)] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		if _, ok := allowedOrigins[origin]; ok {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Vary", "Origin")
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Set("Access-Control-Allow-Headers",
				"Content-Type, Authorization, X-Requested-With")
			c.Writer.Header().Set("Access-Control-Allow-Methods",
				"GET, POST, PUT, PATCH, DELETE, OPTIONS")
			c.Writer.Header().Set("Access-Control-Max-Age", "86400")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
