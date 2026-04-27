package middleware

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method
		ip := c.ClientIP()
		ua := c.Request.UserAgent()

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		errs := c.Errors.ByType(gin.ErrorTypePrivate).String()

		if errs != "" {
			log.Printf("[ERROR] %s | %3d | %13v | %15s | %-7s %s | %s",
				start.Format(time.RFC3339),
				status,
				latency,
				ip,
				method,
				path,
				errs,
			)
			return
		}

		log.Printf("[INFO] %s | %3d | %13v | %15s | %-7s %s | UA:%s",
			start.Format(time.RFC3339),
			status,
			latency,
			ip,
			method,
			path,
			ua,
		)
	}
}
