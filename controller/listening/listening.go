package listening

import (
	"strconv"

	"github.com/eric2788/biligo-live-ws/services/blive"
	"github.com/gin-gonic/gin"
)

func Register(gp *gin.RouterGroup) {
	gp.GET("", GetListening)
	gp.GET("/:room_id", GetListenRoom)
}

func GetListenRoom(c *gin.Context) {

	id, err := strconv.ParseInt(c.Param("room_id"), 10, 64)

	if err != nil {
		c.IndentedJSON(400, gin.H{
			"error": err.Error(),
		})
		return
	}

	room, err := blive.GetListeningInfo(id)

	if err != nil {

		if err == blive.ErrNotFound {
			c.IndentedJSON(404, gin.H{
				"error": "房间不存在",
			})
			return
		}

		c.IndentedJSON(400, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.IndentedJSON(200, room)
}

func GetListening(c *gin.Context) {

	listens := blive.GetEntered()

	c.JSON(200, gin.H{
		"total_started_count":   len(blive.GetListening()),
		"excepted_count":        len(blive.GetExcepted()),
		"total_listening_count": len(listens),
		"rooms":                 listens,
	})
}
