package blive

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	set "github.com/deckarep/golang-set"
	"github.com/eric2788/biligo-live-ws/services/api"
	"github.com/gorilla/websocket"
	biligo "github.com/iyear/biligo-live"
)

var (
	listening          = set.NewSet()
	shortRoomListening = set.NewSet()
	excepted           = set.NewSet()
	liveFetch          = set.NewSet()
	coolingDown        = set.NewSet()

	enteredRooms = set.NewSet()

	ShortRoomMap = sync.Map{}

	heartBeatMap = sync.Map{}
)

var (
	ErrNotFound = errors.New("房间不存在")
	ErrTooFast  = errors.New("请求频繁")
)

func GetExcepted() []interface{} {
	return excepted.ToSlice()
}

func GetEntered() []interface{} {
	return enteredRooms.ToSlice()
}

func GetListening() []interface{} {
	return listening.ToSlice()
}

func coolDownLiveFetch(room int64) {
	liveFetch.Add(room)
	<-time.After(time.Minute * 5)
	liveFetch.Remove(room)
}

func LaunchLiveServer(
	wg *sync.WaitGroup,
	room int64,
	handle func(data *LiveInfo, msg biligo.Msg),
	finished func(context.CancelFunc, error),
) {

	defer wg.Done()

	log.Debugf("[%v] 正在获取直播资讯...", room)

	liveInfo, err := GetLiveInfo(room) // 获取直播资讯

	if err != nil {

		if err == ErrTooFast {
			// 假設为已添加监听以防止重複监听
			coolingDown.Add(room)
			go func() {
				cool := time.Minute*10 + time.Second*time.Duration(len(coolingDown.ToSlice()))
				log.Warnf("将于 %v 后再尝试监听直播: %d", shortDur(cool), room)
				// 十分钟冷却后再重試
				<-time.After(cool)
				coolingDown.Remove(room)
			}()
		}

		log.Errorf("[%v] 获取直播资讯失敗: %v", room, err)
		finished(nil, err)
		return
	}

	log.Debugf("[%v] 获取直播资讯成功。", room)

	realRoom := liveInfo.RoomId

	// 监听房间为短号
	if room != realRoom {

		// 添加到映射
		ShortRoomMap.Store(realRoom, room)

		// 真正房间号已经在监听
		if listening.Contains(realRoom) {
			log.Infof("检测到 %v 为短号，真正房间号为 %v 且正在监听中。", room, realRoom)
			shortRoomListening.Add(room)
			finished(nil, nil)
			return
		}

	}

	live := biligo.NewLive(false, 30*time.Second, 2048, func(err error) {
		log.Error(err)
	})

	var wsHost = biligo.WsDefaultHost

	// 如果有强制指定 ws host, 則使用
	if strings.HasPrefix(os.Getenv("BILI_WS_HOST_FORCE"), "wss://") {

		wsHost = os.Getenv("BILI_WS_HOST_FORCE")

	} else if os.Getenv("BILI_WS_HOST_FORCE") == "AUTO" { // 否則从 api 获取 host list 並提取低延迟

		lowHost := api.GetLowLatencyHost(realRoom, false)

		if lowHost == "" {
			log.Warnf("[%v] 无法获取低延迟 Host，将使用预设 Host", realRoom)
		} else {
			log.Debugf("[%v] 已采用 %v 作为低延迟 Host", realRoom, lowHost)
			wsHost = lowHost
		}

	} // 否則繼續使用 biligo.WsDefaultHost

	log.Debugf("[%v] 已采用 %v 作为直播 Host", realRoom, wsHost)

	log.Debugf("[%v] 正在连接到弹幕伺服器...", room)

	if err := live.Conn(websocket.DefaultDialer, wsHost); err != nil {
		log.Warn("连接伺服器时出现错误: ", err)
		finished(nil, err)
		return
	}

	log.Debugf("[%v] 连接到弹幕伺服器成功。", room)

	ctx, stop := context.WithCancel(context.Background())

	go func() {

		if err := live.Enter(ctx, realRoom, "", 0); err != nil {
			log.Warnf("监听房间 %v 时出现错误: %v\n", realRoom, err)
			stop()
		}

	}()

	go func() {

		enteredRooms.Add(realRoom)
		defer enteredRooms.Remove(realRoom)

		hbCtx, hbCancel := context.WithCancel(ctx)
		// 在启动监听前先启动一次heartbeat监听
		go listenHeartBeatExpire(realRoom, time.Now(), stop, hbCtx)

		for {
			select {
			case tp := <-live.Rev:
				if tp.Error != nil {
					log.Error(tp.Error)
					continue
				}
				// 开播 !?
				if _, ok := tp.Msg.(*biligo.MsgLive); ok {

					// 更新直播资讯只做一次
					if !liveFetch.Contains(realRoom) {
						go coolDownLiveFetch(realRoom)
						log.Infof("房间 %v 开播，正在更新直播资讯...\n", realRoom)
						// 更新一次直播资讯
						UpdateLiveInfo(liveInfo, realRoom)
						// 更新一次 WebSocket 资讯
						go api.UpdateLowLatencyHost(realRoom)
					}

					// 但开播指令推送多次保留
				}
				// 使用懸掛防止下一個訊息阻塞等待
				go handle(liveInfo, tp.Msg)
				go save_danmaku(tp.Msg.Cmd(), liveInfo, tp.Msg)

				// 記錄上一次接收到 Heartbeat 的时間
				if _, ok := tp.Msg.(*biligo.MsgHeartbeatReply); ok {
					go listenHeartBeatExpire(realRoom, time.Now(), stop, hbCtx)
				}

			case <-ctx.Done():
				log.Infof("房间 %v 监听中止。\n", realRoom)
				hbCancel()
				finished(nil, nil)
				if realRoom != room {
					listening.Remove(realRoom)
					shortRoomListening.Remove(room)
				}
				return
			}
		}
	}()

	if room != realRoom {
		log.Infof("%v 为短号，已新增真正的房间号 %v => %v 作为监听。", room, room, realRoom)
		shortRoomListening.Add(room)
		listening.Add(realRoom)
	}

	finished(stop, nil)
}

func listenHeartBeatExpire(realRoom int64, lastListen time.Time, stop context.CancelFunc, ctx context.Context) {
	heartBeatMap.Store(realRoom, lastListen)
	select {
	case <-time.After(time.Minute):
		break
	case <-ctx.Done(): // 已终止监听
		return
	}
	// 一分钟后 heartbeat 依然相同
	if lastTime, ok := heartBeatMap.Load(realRoom); ok && (lastTime.(time.Time)).Equal(lastListen) {
		log.Warnf("房间 %v 在一分钟后依然沒有收到新的 HeartBeat, 已强制终止目前的监听。", realRoom)
		stop() // 調用中止监听
	}
}

func shortDur(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = s[:len(s)-2]
	}
	if strings.HasSuffix(s, "h0m") {
		s = s[:len(s)-2]
	}
	return s
}
