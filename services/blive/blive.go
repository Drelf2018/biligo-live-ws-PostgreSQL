package blive

import (
	"context"
	"sync"
	"time"

	"github.com/eric2788/biligo-live-ws/services/subscriber"
	live "github.com/iyear/biligo-live"
	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("service", "blive")
var stopMap = sync.Map{}

func SubscribedRoomTracker(handleWs func(int64, *LiveInfo, live.Msg)) {
	log.Info("已启动房间订阅监听。")
	wg := &sync.WaitGroup{}
	for {
		time.Sleep(time.Second * 5)

		rooms := subscriber.GetAllRooms()

		log.Debug("房间订阅: ", rooms.ToSlice())
		log.Debug("正在监听: ", listening.ToSlice())

		for toListen := range rooms.Difference(listening).Iter() {

			if excepted.Contains(toListen) {
				log.Debugf("房间 %v 已排除", toListen)
				continue
			}
			// 已经启动监听的短号
			if shortRoomListening.Contains(toListen) {
				log.Debugf("房间 %v 已经启动短号监听", toListen)
				continue
			}

			// 冷却时暂不监听直播
			if coolingDown.Contains(toListen) {
				log.Debugf("房间 %v 在冷却时暂不监听直播", toListen)
				continue
			}

			room := toListen.(int64)

			log.Info("正在启动监听房间: ", room)

			wg.Add(1)
			go LaunchLiveServer(wg, room,
				func(data *LiveInfo, msg live.Msg) {
					save_danmaku(msg.Cmd(), data, msg)
					handleWs(room, data, msg)
				}, func(stop context.CancelFunc, err error) {
					if err == nil && stop != nil {
						stopMap.Store(room, stop)
					} else {
						listening.Remove(room)
						if short, ok := ShortRoomMap.Load(room); ok {
							shortRoomListening.Remove(short)
						}
						log.Warnf("已移除房间 %v 的监听状态", room)
					}
				})
			listening.Add(room)
		}

		wg.Wait()

		for short := range shortRoomListening.Iter() {
			rooms.Add(short)
		}

		for toStop := range listening.Difference(rooms).Iter() {
			room := toStop.(int64)

			if stop, ok := stopMap.LoadAndDelete(room); ok {
				log.Info("正在中止监听房间: ", room)
				stop.(context.CancelFunc)()
			}
		}

	}
}
