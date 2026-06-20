package middleware

import (
	"net/http"

	"github.com/Refliqx/backend-eletter/internal/response"
	"github.com/gin-gonic/gin"
)

func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole := c.GetString("userRole")
		for _, r := range roles {
			if userRole == r {
				c.Next()
				return
			}
		}
		response.Error(c, http.StatusForbidden, "Akses ditolak: peran tidak diizinkan")
		c.Abort()
	}
}
