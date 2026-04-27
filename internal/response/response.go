package response

import "github.com/gin-gonic/gin"

func JSON(c *gin.Context, statusCode int, success bool, message string, data interface{}, errMsg string) {
	c.JSON(statusCode, gin.H{
		"success": success,
		"message": message,
		"data":    data,
		"error":   errMsg,
	})
}

func Success(c *gin.Context, statusCode int, message string, data interface{}) {
	JSON(c, statusCode, true, message, data, "")
}

func Error(c *gin.Context, code int, message string) {
	JSON(c, code, false, message, nil, message)
}

func Raw(c *gin.Context, statusCode int, payload gin.H) {
	c.JSON(statusCode, payload)
}

func LegacySuccess(c *gin.Context, message string, data interface{}) {
	c.JSON(200, gin.H{
		"success": true,
		"message": message,
		"data":    data,
	})
}

func LegacyError(c *gin.Context, code int, message string) {
	c.JSON(code, gin.H{
		"success": false,
		"message": message,
		"data":    nil,
	})
}
