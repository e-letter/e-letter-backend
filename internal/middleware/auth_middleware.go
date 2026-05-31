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
		var token string
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		} else {
			token = c.Query("token")
		}

		if token == "" {
			response.Error(c, http.StatusUnauthorized, "Token akses diperlukan")
			c.Abort()
			return
		}

		claims, err := utils.ParseAndValidateToken(token, jwtSecret, "access")
		if err != nil {
			response.Error(c, http.StatusUnauthorized, "Token tidak valid")
			c.Abort()
			return
		}

		c.Set("userId", claims.UserID)
		c.Set("userEmail", claims.Email)
		c.Set("userRole", claims.Role)
		c.Set("subRoles", claims.SubRoles)
		c.Next()
	}
}
