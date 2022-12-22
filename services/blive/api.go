package blive

import (
	"errors"

	"github.com/eric2788/biligo-live-ws/services/api"
)

func GetListeningInfo(room int64) (*ListeningInfo, error) {

	liveInfo, err := GetLiveInfoCache(room)
	if err != nil {
		return nil, err
	}

	userInfo, err := api.GetUserInfoCache(liveInfo.UID)

	if err != nil {
		return nil, err
	}

	// 先前沒有記錄
	role := -1

	if userInfo.Data.Official != nil {
		role = userInfo.Data.Official.Role
	}

	return &ListeningInfo{
		LiveInfo:     liveInfo,
		OfficialRole: role,
	}, nil
}

// UpdateLiveInfo 刷新直播资讯，强制更新緩存
func UpdateLiveInfo(info *LiveInfo, room int64) {

	latestRoomInfo, err := api.GetRoomInfoWithOption(room, true)

	// 房间资讯请求成功
	if err == nil && latestRoomInfo.Code == 0 {
		// 更新房间资讯
		info.Cover = latestRoomInfo.Data.UserCover
		info.Title = latestRoomInfo.Data.Title
		info.UID = latestRoomInfo.Data.Uid
		log.Debugf("房间直播资讯 %v 刷新成功。", room)
	} else {
		if err != nil {
			log.Warnf("房间直播资讯 %v 刷新失败: %v", room, err)
		} else {
			log.Warnf("房间直播资讯 %v 刷新失败: %v", room, latestRoomInfo.Message)
		}
	}

	latestUserInfo, err := api.GetUserInfo(info.UID, true)
	// 用户资讯请求成功
	if err == nil && latestUserInfo.Code == 0 {
		// 更新用户资讯
		info.Name = latestUserInfo.Data.Name
		info.UserFace = latestUserInfo.Data.Face
		info.UserDescription = latestUserInfo.Data.Sign

		log.Debugf("房间用户资讯 %v 刷新成功。", info.UID)
	} else {
		if err != nil {
			log.Warnf("房间用户资讯 %v 刷新失败: %v", info.UID, err)
		} else {
			log.Warnf("房间用户资讯 %v 刷新失败: %v", info.UID, latestUserInfo.Message)
		}
	}
}

// GetLiveInfoCache 从緩存提取，不存在則返回 ErrCacheNotFound
func GetLiveInfoCache(room int64) (*LiveInfo, error) {

	info, err := api.GetRoomInfoCache(room)

	if err != nil {
		return nil, err
	}

	data := info.Data
	user, err := api.GetUserInfoCache(data.Uid)

	if err != nil {
		return nil, err
	}

	liveInfo := &LiveInfo{
		RoomId:          info.Data.RoomId,
		UID:             data.Uid,
		Title:           data.Title,
		Name:            user.Data.Name,
		Cover:           data.UserCover,
		UserFace:        user.Data.Face,
		UserDescription: user.Data.Sign,
	}

	return liveInfo, nil
}

// GetLiveInfo 获取直播资讯，不强制更新緩存
func GetLiveInfo(room int64) (*LiveInfo, error) {

	// 已在 exception 內, 則返回不存在
	if excepted.Contains(room) {
		return nil, ErrNotFound
	}

	info, err := api.GetRoomInfoWithOption(room, false)

	if err != nil {
		log.Warnf("索取房间资讯 %v 时出现错误: %v", room, err)
		return nil, err
	}

	// 房间资讯请求过快被拦截
	if info.Code == -412 {
		log.Warnf("错误: 房间 %v 请求频繁被拦截", room)
		return nil, ErrTooFast
	}

	// 未找到该房间
	if info.Code == 1 {
		log.Warnf("房间不存在 %v", room)
		excepted.Add(room)
		return nil, ErrNotFound
	}

	if info.Data == nil {
		log.Warnf("索取房间资讯 %v 时出现错误: %v", room, info.Message)
		excepted.Add(room)
		return nil, errors.New(info.Message)
	}

	data := info.Data
	user, err := api.GetUserInfo(data.Uid, false)

	if err != nil {
		log.Warn("索取用户资讯时出现错误: ", err)
		return nil, err
	}

	// 用户资讯请求过快被拦截
	if user.Code == -412 {
		log.Warnf("错误: 用户 %v 请求频繁被拦截", data.Uid)
		return nil, ErrTooFast
	}

	if user.Data == nil {
		log.Warn("索取用户资讯时出现错误: ", user.Message)
		// 404 not found
		if user.Code == -404 {
			log.Warnf("用户 %v 不存在，已排除该房间。", data.Uid)
			excepted.Add(room)
			return nil, ErrNotFound
		}
		return nil, errors.New(user.Message)
	}

	liveInfo := &LiveInfo{
		RoomId:          info.Data.RoomId,
		UID:             data.Uid,
		Title:           data.Title,
		Name:            user.Data.Name,
		Cover:           data.UserCover,
		UserFace:        user.Data.Face,
		UserDescription: user.Data.Sign,
	}

	return liveInfo, nil

}
