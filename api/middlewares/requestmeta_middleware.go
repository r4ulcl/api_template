package middlewares

import (
	"github.com/google/uuid"

	"github.com/gin-gonic/gin"
)

func RequestMeta() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader("X-Request-ID")
		if rid == "" {
			rid = uuid.New().String()
		}
		c.Set("request_id", rid)
		c.Writer.Header().Set("X-Request-ID", rid)

		c.Next()
	}
}
