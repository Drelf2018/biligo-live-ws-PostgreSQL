package blive

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	live "github.com/iyear/biligo-live"
	_ "github.com/lib/pq"
)

var db *sql.DB
var live_stmt, stop_stmt *sql.Stmt
var ROOM_STATUS = make(map[int64]int64)
var SUPER_CHAT = make(map[int64]struct{})
var DanmakuData []Danmaku

func init() {
	f, err := os.Open("database.txt")
	if err != nil {
		return
	}
	var data []byte
	buf := make([]byte, 1024)
	for {
		// 将文件中读取的byte存储到buf中
		n, err := f.Read(buf)
		if err != nil && err != io.EOF {
			log.Fatal(err)
		}
		if n == 0 {
			break
		}
		// 将读取到的结果追加到data切片中
		data = append(data, buf[:n]...)
	}

	db, err = sql.Open("postgres", string(data))
	if err == nil {
		live_stmt, _ = db.Prepare("INSERT INTO live(roomid,username,uid,title,cover,st) VALUES($1,$2,$3,$4,$5,$6)")
		stop_stmt, _ = db.Prepare("update live set sp=$1 where roomid=$2 and st=$3")
		query()

		go func() {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()

			for range ticker.C {
				auto_save()
			}
		}()
	} else {
		log.Error("打开数据库时错误。", err)
		db = nil
	}
}

type Status struct {
	roomid int64
	st     int64
}

type Danmaku struct {
	roomid int64
	time   int64
	uname  string
	mid    int64
	msg    string
	cmd    string
	price  float64
	st     int64
}

func query() {
	rows, err := db.Query("select roomid, st from live where sp is NULL")
	if err != nil {
		log.Error("从数据库读取房间状态失败。", err)
		return
	}
	//延迟关闭rows
	defer rows.Close()

	for rows.Next() {
		status := Status{}
		err := rows.Scan(&status.roomid, &status.st)
		if err != nil {
			panic(err)
		}
		ROOM_STATUS[status.roomid] = status.st
	}
}

func insert_danmaku(roomid, time, mid int64, price float64, uname, msg, cmd string) {
	start_time, ok := ROOM_STATUS[roomid]
	if !ok {
		start_time = 0
	}

	DanmakuData = append(DanmakuData, Danmaku{roomid, time, uname, mid, msg, cmd, price, start_time})
}

func auto_save() {
	sql := "INSERT INTO danmaku(roomid,time,username,uid,msg,cmd,price,st) VALUES "
	var tData []Danmaku
	pos := len(DanmakuData)
	tData, DanmakuData = DanmakuData[:pos], DanmakuData[pos:]
	if pos > 0 {
		for k, v := range tData {
			if k == 0 {
				sql += fmt.Sprintf("(%v,%v,'%v',%v,'%v','%v',%v,%v)", v.roomid, v.time, v.uname, v.mid, v.msg, v.cmd, v.price, v.st)
			} else {
				sql += fmt.Sprintf(",(%v,%v,'%v',%v,'%v','%v',%v,%v)", v.roomid, v.time, v.uname, v.mid, v.msg, v.cmd, v.price, v.st)
			}
		}
		res, err := db.Exec(sql)
		if err != nil {
			log.Error("保存弹幕到数据库时错误。", err)
		} else {
			line, _ := res.RowsAffected()
			log.Error("保存弹幕到数据库成功。条目数: ", line)
		}
	}
}

func save_danmaku(Cmd string, live_info *LiveInfo, msg live.Msg) {
	if db == nil {
		return
	}
	switch msg := msg.(type) {
	case *live.MsgLive:
		now := time.Now().Unix()
		_, ok := ROOM_STATUS[live_info.RoomId]
		if !ok {
			ROOM_STATUS[live_info.RoomId] = now
			live_stmt.Exec(live_info.RoomId, live_info.Name, live_info.UID, live_info.Title, live_info.Cover, now)
		}

	case *live.MsgDanmaku:
		dm, err := msg.Parse()
		if err == nil {
			insert_danmaku(live_info.RoomId, dm.Time/1000, dm.MID, 0.0, dm.Uname, dm.Content, Cmd)
		} else {
			panic(err)
		}

	case *live.MsgSendGift:
		dm, err := msg.Parse()
		if err == nil {
			msg := fmt.Sprintf("%s %s<font color=\"red\">￥%.2f</font>", dm.Action, dm.GiftName, float64(dm.Price)/1000.0)
			insert_danmaku(live_info.RoomId, dm.Timestamp, dm.UID, float64(dm.Price)/1000.0, dm.Uname, msg, Cmd)
		} else {
			panic(err)
		}

	case *live.MsgGuardBuy:
		dm, err := msg.Parse()
		if err == nil {
			msg := fmt.Sprintf("赠送 %s<font color=\"red\">￥%.2f</font>", dm.GiftName, float64(dm.Price)/1000.0)
			insert_danmaku(live_info.RoomId, dm.StartTime, dm.UID, float64(dm.Price)/1000.0, dm.Username, msg, Cmd)
		} else {
			panic(err)
		}

	case *live.MsgSuperChatMessage:
		dm, err := msg.Parse()
		if err == nil {
			_, ok := SUPER_CHAT[dm.ID]
			if ok {
				return
			} else {
				SUPER_CHAT[dm.ID] = struct{}{}
				msg := fmt.Sprintf("%s<font color=\"red\">￥%d</font>", dm.Message, dm.Price)
				insert_danmaku(live_info.RoomId, dm.StartTime, dm.UID, float64(dm.Price), dm.UserInfo.Uname, msg, Cmd)
			}
		}

	case *live.MsgSuperChatMessageJPN:
		dm, err := msg.Parse()
		if err == nil {
			JpnID, err := strconv.ParseInt(dm.ID, 10, 64)
			if err == nil {
				_, ok := SUPER_CHAT[JpnID]
				if ok {
					return
				} else {
					SUPER_CHAT[JpnID] = struct{}{}
					msg := fmt.Sprintf("%s<font color=\"red\">￥%d</font>", dm.Message, dm.Price)
					JpnUID, err := strconv.ParseInt(dm.UID, 10, 64)
					if err == nil {
						insert_danmaku(live_info.RoomId, dm.StartTime, JpnUID, float64(dm.Price), dm.UserInfo.Uname, msg, "SUPER_CHAT_MESSAGE")
					}
				}
			}
		}

	case *live.MsgPreparing:
		now := time.Now().Unix()
		st := ROOM_STATUS[live_info.RoomId]
		delete(ROOM_STATUS, live_info.RoomId)
		stop_stmt.Exec(now, live_info.RoomId, st)
	}
}
