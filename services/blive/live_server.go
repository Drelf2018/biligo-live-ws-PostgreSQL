package blive

import (
	"context"
	"errors"
	set "github.com/deckarep/golang-set"
	biligo "github.com/eric2788/biligo-live"
	"github.com/eric2788/biligo-live-ws/services/api"
	"github.com/gorilla/websocket"
	"log"
	"time"
)

var listening = set.NewSet()
var excepted = set.NewSet()

func LaunchLiveServer(room int64, handle func(data *LiveInfo, msg biligo.Msg)) (context.CancelFunc, error) {

	liveInfo, err := GetLiveInfo(room, false) // 獲取直播資訊

	if err != nil {
		return nil, err
	}

	live := biligo.NewLive(true, 30*time.Second, 0, func(err error) {
		log.Fatal(err)
	})

	if err := live.Conn(websocket.DefaultDialer, biligo.WsDefaultHost); err != nil {
		log.Println("連接伺服器時出現錯誤: ", err)
		return nil, err
	}

	ctx, stop := context.WithCancel(context.Background())

	go func() {

		if err := live.Enter(ctx, room, "", 0); err != nil {
			log.Printf("監聽房間 %v 時出現錯誤: %v\n", room, err)
			listening.Remove(room)
			stop()
		}

	}()

	go func() {
		for {
			select {
			case tp := <-live.Rev:
				if tp.Error != nil {
					log.Println(tp.Error)
					continue
				}
				// 開播 !?
				if _, ok := tp.Msg.(*biligo.MsgLive); ok {
					log.Printf("房間 %v 開播，正在更新直播資訊...\n", room)
					// 更新一次直播资讯
					if latestInfo, err := GetLiveInfo(room, true); err == nil {
						// 更新成功， 更新
						liveInfo = latestInfo
					}
				}
				handle(liveInfo, tp.Msg)
			case <-ctx.Done():
				log.Printf("房間 %v 監聽中止。\n", room)
				listening.Remove(room)
				return
			}
		}
	}()

	listening.Add(room)
	return stop, nil
}

type LiveInfo struct {
	RoomId int64  `json:"room_id"`
	UID    int64  `json:"uid"`
	Title  string `json:"title"`
	Name   string `json:"name"`
	Cover  string `json:"cover"`
}

func GetLiveInfo(room int64, forceUpdate bool) (*LiveInfo, error) {

	info, err := api.GetRoomInfoWithOption(room, forceUpdate)

	if err != nil {
		log.Println("索取房間資訊時出現錯誤: ", err)
		return nil, err
	}

	if info.Data == nil {
		log.Println("索取房間資訊時出現錯誤: ", info.Message)
		excepted.Add(room)
		return nil, errors.New(info.Message)
	}

	data := info.Data
	user, err := api.GetUserInfo(data.Uid, forceUpdate)

	if err != nil {
		log.Println("索取用戶資訊時出現錯誤: ", err)
		return nil, err
	}

	if user.Data == nil {
		log.Println("索取用戶資訊時出現錯誤: ", err)
		return nil, errors.New(info.Message)
	}

	liveInfo := &LiveInfo{
		RoomId: room,
		UID:    data.Uid,
		Title:  data.Title,
		Name:   user.Data.Name,
		Cover:  data.UserCover,
	}

	return liveInfo, nil

}
