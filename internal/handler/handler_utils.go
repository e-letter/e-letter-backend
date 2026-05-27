package handler

import "github.com/gin-gonic/gin"

func toIntFromContext(c *gin.Context, key string) int {
	v, ok := c.Get(key)
	if !ok {
		return 0
	}
	if floatVal, isFloat := v.(float64); isFloat {
		return int(floatVal)
	}
	if intVal, isInt := v.(int); isInt {
		return intVal
	}
	return 0
}
