package subscriber

import (
	"fmt"
	"sync"
	"time"

	set "github.com/deckarep/golang-set"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

var (
	queue        = set.NewSet()
	subscribeMap = sync.Map{}
	expireMap    = sync.Map{}
	log          = logrus.WithField("service", "subscriber")
)

// Update 操作太慢，尝试使用 go 懸掛
func Update(identifier string, rooms []int64) {
	log.Infof("%v 的订阅更新已加入队列...", identifier)
	queue.Add(identifier)
	go func() {
		subscribeMap.Store(identifier, rooms)
		log.Infof("%v 的订阅更新已完成。", identifier)
		queue.Remove(identifier)
	}()
}

func ExpireAfter(identifier string, expired <-chan time.Time) {
	ExpireAfterWithCheck(identifier, expired, true)
}

func ExpireAfterWithCheck(identifier string, expired <-chan time.Time, checkExist bool) {

	// 保險起見
	if _, subBefore := subscribeMap.Load(identifier); subBefore && checkExist {
		return
	}

	// 队列內有，防止过期
	if checkExist && queue.Contains(identifier) {
		return
	}

	connected := make(chan struct{})

	go func() {
		for {
			select {
			case <-expired:
				// 保險起見
				if _, ok := expireMap.LoadAndDelete(identifier); !ok {
					return
				}
				log.Infof("%v 的订阅已过期。\n", identifier)
				subscribeMap.Delete(identifier)
				return
			case <-connected:
				log.Infof("已终止用户 %v 的订阅过期。", identifier)
				return
			}
		}
	}()

	expireMap.Store(identifier, connected)
	log.Infof("已启动用户 %v 的订阅过期。", identifier)
}

var void struct{}

func CancelExpire(identifier string) {
	if connected, ok := expireMap.LoadAndDelete(identifier); ok {
		conn := connected.(chan struct{})
		conn <- void
	}
}

func Get(identifier string) ([]int64, bool) {
	if res, ok := subscribeMap.Load(identifier); ok {
		return res.([]int64), ok
	} else {
		return nil, ok
	}
}

func GetOrEmpty(identifier string) ([]int64, bool) {
	res, ok := Get(identifier)
	if !ok {
		res = []int64{}
	}
	return res, ok
}

func Poll(identifier string) ([]int64, bool) {
	if res, ok := subscribeMap.LoadAndDelete(identifier); ok {
		return res.([]int64), ok
	} else {
		return nil, ok
	}
}

func GetAllRooms() set.Set {
	rooms := set.NewSet()
	subscribeMap.Range(func(key, value interface{}) bool {
		for _, room := range value.([]int64) {
			rooms.Add(room)
		}
		return true
	})
	return rooms
}

func GetAllSubscribers(room int64) []string {
	identifiers := make([]string, 0)
	subscribeMap.Range(func(identifier, rooms interface{}) bool {
		for _, rm := range rooms.([]int64) {
			if room == rm {
				identifiers = append(identifiers, identifier.(string))
				break
			}
		}
		return true
	})

	return identifiers
}

func Add(identifier string, rooms []int64) []int64 {

	res, ok := Get(identifier)

	if !ok {
		res = make([]int64, 0)
	}

	newRooms := UpdateRange(res, rooms, func(s set.Set, i int64) {
		s.Add(i)
	})

	Update(identifier, newRooms)
	return newRooms
}

func UpdateRange(res []int64, rooms []int64, updater func(set.Set, int64)) []int64 {

	roomSet := ToSet(res)

	for _, room := range rooms {
		updater(roomSet, room)
	}

	roomArr := roomSet.ToSlice()
	newRooms := make([]int64, len(roomArr))
	for i, room := range roomArr {
		newRooms[i] = room.(int64)
	}

	return newRooms
}

func Remove(identifier string, rooms []int64) ([]int64, bool) {

	res, ok := Get(identifier)

	if !ok {
		return nil, false
	}

	newRooms := UpdateRange(res, rooms, func(s set.Set, i int64) {
		s.Remove(i)
	})

	Update(identifier, newRooms)
	return newRooms, true
}

func Delete(identifier string) {
	subscribeMap.Delete(identifier)
}

func ToSet(arr []int64) set.Set {
	s := set.NewThreadUnsafeSet()
	for _, k := range arr {
		s.Add(k)
	}
	return s
}

func ToClientId(c *gin.Context) string {
	identifier := c.GetHeader("Authorization")
	if identifier == "" {
		identifier = "anonymous"
	}
	return fmt.Sprintf("%v@%v", c.ClientIP(), identifier)
}
