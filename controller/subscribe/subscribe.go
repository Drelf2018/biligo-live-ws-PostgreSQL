package subscribe

import (
	"strconv"
	"time"

	mapset "github.com/deckarep/golang-set"
	"github.com/eric2788/biligo-live-ws/services/api"
	"github.com/eric2788/biligo-live-ws/services/subscriber"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

var (
	Id  = subscriber.ToClientId
	log = logrus.WithField("controller", "subscribe")
)

func Register(gp *gin.RouterGroup) {
	gp.GET("", GetSubscriptions)
	gp.POST("", Subscribe)
	gp.DELETE("", ClearSubscribe)
	gp.PUT("add", AddSubscribe)
	gp.PUT("remove", RemoveSubscribe)
}

func GetSubscriptions(c *gin.Context) {
	list, ok := subscriber.Get(Id(c))
	if !ok {
		list = []int64{}
	}
	c.IndentedJSON(200, list)
}

func ClearSubscribe(c *gin.Context) {
	subscriber.Delete(Id(c))
	c.Status(200)
}

func AddSubscribe(c *gin.Context) {

	dontCheck := c.Query("validate") == "false" // 是否不检查房间讯息
	rooms, ok := GetSubscribesArr(c, !dontCheck)

	if !ok {
		return
	}

	log.Infof("用户 %v 新增订阅 %v \n", Id(c), rooms)

	ActivateExpire(Id(c))

	newRooms := subscriber.Add(Id(c), rooms)
	c.IndentedJSON(200, newRooms)
}

func RemoveSubscribe(c *gin.Context) {

	rooms, ok := GetSubscribesArr(c, false) // 刪除订阅不检查房间訊息是否存在

	if !ok {
		return
	}

	log.Infof("用户 %v 移除订阅 %v \n", Id(c), rooms)

	newRooms, ok := subscriber.Remove(Id(c), rooms)

	if !ok {
		c.IndentedJSON(400, gin.H{"error": "删除失败，你尚未提交过任何订阅"})
		return
	}

	c.IndentedJSON(200, newRooms)
}

func Subscribe(c *gin.Context) {
	dontCheck := c.Query("validate") == "false" // 是否不检查房间讯息
	rooms, ok := GetSubscribesArr(c, !dontCheck)

	if !ok {
		return
	}

	log.Infof("用户 %v 设置订阅 %v \n", Id(c), rooms)

	ActivateExpire(Id(c))

	subscriber.Update(Id(c), rooms)
	c.IndentedJSON(200, rooms)
}

func GetSubscribesArr(c *gin.Context, checkExist bool) ([]int64, bool) {

	subArr, ok := c.GetPostFormArray("subscribes")
	if !ok {
		c.AbortWithStatusJSON(400, gin.H{"error": "缺少 `subscribes` 数值(订阅列表)"})
		return nil, false
	}
	if len(subArr) == 0 {
		c.AbortWithStatusJSON(400, gin.H{"error": "订阅列表不能为空"})
		return nil, false
	}

	roomSet := mapset.NewSet[int64]()

	for _, arr := range subArr {

		roomId, err := strconv.ParseInt(arr, 10, 64)

		if err != nil {
			log.Warn("cannot parse room: ", err.Error())
			continue
		}

		if checkExist {

			realRoom, roomErr := api.GetRealRoom(roomId)

			if roomErr != nil {
				log.Warnf("获取房间讯息时出现错误: %v", roomErr)
				_ = c.Error(roomErr)
				return nil, false
			} else {
				if realRoom > 0 {
					roomSet.Add(realRoom)
				} else {
					log.Warnf("房间 %v 无效，已过滤 \n", roomId)
				}
			}
		} else {
			roomSet.Add(roomId)
		}

	}

	// 有生之年我居然還要用 loop 轉泛型 array 同 type array, 醉了
	roomArr := roomSet.ToSlice()
	rooms := make([]int64, len(roomArr))

	for i, v := range roomArr {
		rooms[i] = v
	}

	return rooms, true
}

func ActivateExpire(identifier string) {
	// 如果之前尚未有过订阅 (即新增而不是更新)
	if _, subBefore := subscriber.Get(identifier); !subBefore {
		// 設置如果五分钟后尚未连線 WebSocket 就清除订阅記憶
		subscriber.ExpireAfter(identifier, time.NewTimer(time.Minute*5))
	}
}
