package middleware

import (
	"net/http"
	"strings"

	"github.com/Refliqx/backend-eletter/internal/response"
	"github.com/Refliqx/backend-eletter/internal/utils"
	"github.com/gin-gonic/gin"
)

func RequireAccessToken(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			response.Error(c, http.StatusUnauthorized, "Token akses diperlukan")
			c.Abort()
			return
		}

		token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		claims, err := utils.ParseAndValidateToken(token, jwtSecret, "access")
		if err != nil {
			response.Error(c, http.StatusUnauthorized, "Token tidak valid")
			c.Abort()
			return
		}

		c.Set("userId", claims.UserID)
		c.Set("userEmail", claims.Email)
		c.Set("userRole", claims.Role)
		c.Next()
	}
}
