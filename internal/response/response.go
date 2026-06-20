package response

import "github.com/gin-gonic/gin"

type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
}

func Success(c *gin.Context, statusCode int, message string, data interface{}) {
	c.JSON(statusCode, Response{
		Success: true,
		Message: message,
		Data:    data,
	})
}

func Error(c *gin.Context, statusCode int, errorMessage string) {
	c.JSON(statusCode, Response{
		Success: false,
		Error:   errorMessage,
	})
}

func ErrorWithMessage(c *gin.Context, statusCode int, errorMessage string, message string) {
	c.JSON(statusCode, Response{
		Success: false,
		Error:   errorMessage,
		Message: message,
	})
}

func Raw(c *gin.Context, statusCode int, payload gin.H) {
	c.JSON(statusCode, payload)
}
