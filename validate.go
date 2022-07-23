package main

import (
	"github.com/eric2788/biligo-live-ws/services/subscriber"
	"github.com/gin-gonic/gin"
)

func ValidateProcess(c *gin.Context) {
	subs, ok := subscriber.Get(subscriber.ToClientId(c))
	if !ok {
		c.AbortWithStatusJSON(400, gin.H{"error": "尚未订阅任何的直播房间号"})
		return
	}

	if len(subs) == 0 {
		c.AbortWithStatusJSON(400, gin.H{"error": "订阅列表为空"})
		return
	}

	c.Status(200)
}
