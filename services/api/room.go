package api

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/eric2788/biligo-live-ws/services/database"
	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("service", "api")

const RoomInfoApi string = "https://api.live.bilibili.com/room/v1/Room/get_info?room_id=%v"

func GetRoomInfo(room int64) (*RoomInfo, error) {
	return GetRoomInfoWithOption(room, false)
}

func GetRoomInfoCache(room int64) (*RoomInfo, error) {

	dbKey := fmt.Sprintf("room:%v", room)

	var roomInfo = &RoomInfo{}
	if err := database.GetFromDB(dbKey, roomInfo); err == nil {
		return roomInfo, nil
	} else {
		if _, ok := err.(*database.EmptyError); ok {
			return nil, ErrCacheNotFound
		} else {
			return nil, err
		}
	}

}

func GetRoomInfoWithOption(room int64, forceUpdate bool) (*RoomInfo, error) {

	dbKey := fmt.Sprintf("room:%v", room)

	if !forceUpdate {
		if roomInfo, err := GetRoomInfoCache(room); err == nil {
			return roomInfo, nil
		} else {
			if err == ErrCacheNotFound {
				log.Debugf("%v, 正在请求B站 API", err)
			} else {
				log.Warnf("从数据库获取房间资讯 %v 时出现错误: %v, 正在请求B站 API", room, err)
			}
		}
	}

	resp, err := getWithAgent(RoomInfoApi, room)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return nil, err
	}

	var v1resp V1Resp

	if err := json.Unmarshal(body, &v1resp); err != nil {
		return nil, err
	}

	if v1resp.Code != 0 {
		return &RoomInfo{V1Resp: v1resp}, nil
	}

	var roomInfo RoomInfo
	if err := json.Unmarshal(body, &roomInfo); err != nil {
		return nil, err
	}

	roomInfo.Data.UserCover = strings.Replace(roomInfo.Data.UserCover, "http://", "https://", -1)

	if err := database.PutToDB(dbKey, roomInfo); err != nil {
		log.Warnf("从数据库获取房间资讯 %v 时出现错误: %v", room, err)
	} else {
		log.Debugf("房间资讯 %v 更新到数据库成功", room)
	}
	return &roomInfo, nil

}

func GetRealRoom(room int64) (int64, error) {
	res, err := GetRoomInfo(room)

	// 错误
	if err != nil {
		return -1, err
	}

	// 房间不存在
	if res.Data == nil {
		return -1, nil
	}

	return res.Data.RoomId, nil // 返回真实房间号

}
