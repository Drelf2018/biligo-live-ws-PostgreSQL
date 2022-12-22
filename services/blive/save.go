package blive

import (
	"database/sql"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"

	biligo "github.com/eric2788/biligo-live"
	_ "github.com/lib/pq"
)

var db *sql.DB
var live_stmt, stop_stmt *sql.Stmt
var ROOM_STATUS = make(map[int64]int64)
var SUPER_CHAT = make(map[int64]int64)
var DanmakuData []Danmaku
var userKey = flag.String("user", "", "Set PostgreSQL connection")
var pwdKey = flag.String("password", "", "Set PostgreSQL connection")
var dbKey = flag.String("dbname", "", "Set PostgreSQL connection")

func init() {
	// 终于通过延时解决了如何从 flag 中读取字符串的问题
	go func() {
		time.Sleep(1 * time.Second)
		key := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", *userKey, *pwdKey, *dbKey)
		var err error
		db, err = sql.Open("postgres", key)
		if err == nil {
			log.Info("弹幕数据库连接成功。")
			live_stmt, _ = db.Prepare("INSERT INTO live(roomid,username,uid,title,cover,st) VALUES($1,$2,$3,$4,$5,$6)")
			stop_stmt, _ = db.Prepare("update live set sp=$1, total=$2, send_gift=$3, guard_buy=$4, super_chat_message=$5 where roomid=$6 and st=$7")
			rows, err := db.Query("select roomid, st from live where sp is NULL")
			if err != nil {
				log.Error("从弹幕数据库读取房间状态失败。", err)
				return
			}
			//延迟关闭rows
			defer rows.Close()

			if rows != nil {
				for rows.Next() {
					status := Status{}
					err := rows.Scan(&status.roomid, &status.st)
					if err != nil {
						panic(err)
					}
					ROOM_STATUS[status.roomid] = status.st
				}
			}

			go func() {
				ticker := time.NewTicker(10 * time.Second)
				defer ticker.Stop()

				for range ticker.C {
					auto_save()
				}
			}()
		} else {
			log.Error("打开弹幕数据库时错误。", err)
			db = nil
		}
	}()
}

type Status struct {
	roomid int64
	st     int64
}

type Danmaku struct {
	roomid int64
	time   int64
	mid    int64
	uname  string
	msg    string
	price  float64
}

func insert_danmaku(roomid, time, mid int64, price float64, uname, msg string) {
	DanmakuData = append(DanmakuData, Danmaku{roomid, time, mid, uname, msg, price})
}

func auto_save() {
	var tData []Danmaku
	sqlList := make(map[int64]string)
	pos := len(DanmakuData)
	tData, DanmakuData = DanmakuData[:pos], DanmakuData[pos:]
	if pos > 0 {
		for _, v := range tData {
			uname := strings.Replace(v.uname, "'", "''", -1)
			msg := strings.Replace(v.msg, "'", "''", -1)
			sql, ok := sqlList[v.roomid]
			if ok {
				sqlList[v.roomid] = sql + fmt.Sprintf(",(%v,%v,'%v','%v',%v)", v.time, v.mid, uname, msg, v.price)
			} else {
				sqlList[v.roomid] = fmt.Sprintf("INSERT INTO live_%v(time,uid,username,msg,price) VALUES (%v,%v,'%v','%v',%v)", v.roomid, v.time, v.mid, uname, msg, v.price)
			}
		}
		var lines int64 = 0
		for roomid, sql := range sqlList {
			csql := fmt.Sprintf("CREATE TABLE IF NOT EXISTS live_%v(time bigint,uid bigint,username text,msg text,price double precision)", roomid)
			_, err := db.Exec(csql)
			if err != nil {
				log.Error("创建表错误。", err)
			}
			res, err := db.Exec(sql)
			if err != nil {
				log.Error("保存弹幕错误。", err, sql)
			} else {
				line, _ := res.RowsAffected()
				lines += line
			}
		}
		log.Info("保存弹幕成功。总条目数: ", lines)
	}
}

func save_danmaku(Cmd string, live_info *LiveInfo, msg biligo.Msg) {
	if db == nil {
		log.Error("连接到弹幕数据库时错误。")
		return
	}
	switch msg := msg.(type) {
	case *biligo.MsgLive:
		now := time.Now().Unix()
		_, ok := ROOM_STATUS[live_info.RoomId]
		if !ok {
			ROOM_STATUS[live_info.RoomId] = now
			live_stmt.Exec(live_info.RoomId, live_info.Name, live_info.UID, live_info.Title, live_info.Cover, now)
		}

	case *biligo.MsgDanmaku:
		dm, err := msg.Parse()
		if err == nil {
			insert_danmaku(live_info.RoomId, dm.Time/1000, dm.MID, 0.0, dm.Uname, dm.Content)
		} else {
			panic(err)
		}

	case *biligo.MsgSendGift:
		dm, err := msg.Parse()
		if err == nil {
			insert_danmaku(live_info.RoomId, dm.Timestamp, dm.UID, float64(dm.Price)/1000.0, dm.Uname, "投喂 "+dm.GiftName)
		} else {
			panic(err)
		}

	case *biligo.MsgUserToastMsg:
		dm, err := msg.Parse()
		if err == nil {
			insert_danmaku(live_info.RoomId, dm.StartTime, dm.UID, float64(dm.Price)/1000.0, dm.Username, "赠送 "+dm.RoleName)
		} else {
			panic(err)
		}

	case *biligo.MsgSuperChatMessage:
		dm, err := msg.Parse()
		if err == nil {
			_, ok := SUPER_CHAT[dm.ID]
			if ok {
				return
			} else {
				SUPER_CHAT[dm.ID] = dm.ID
				insert_danmaku(live_info.RoomId, dm.StartTime, dm.UID, float64(dm.Price), dm.UserInfo.Uname, dm.Message)
			}
		}

	case *biligo.MsgSuperChatMessageJPN:
		dm, err := msg.Parse()
		if err == nil {
			JpnID, err := strconv.ParseInt(dm.ID, 10, 64)
			if err == nil {
				_, ok := SUPER_CHAT[JpnID]
				if ok {
					return
				} else {
					SUPER_CHAT[JpnID] = JpnID
					JpnUID, err := strconv.ParseInt(dm.UID, 10, 64)
					if err == nil {
						insert_danmaku(live_info.RoomId, dm.StartTime, JpnUID, float64(dm.Price), dm.UserInfo.Uname, dm.Message)
					}
				}
			}
		}

	case *biligo.MsgPreparing:
		now := time.Now().Unix()
		st := ROOM_STATUS[live_info.RoomId]
		delete(ROOM_STATUS, live_info.RoomId)
		time.Sleep(11 * time.Second)
		QuerySql := fmt.Sprintf("select msg,price from live_%v where time >= %v and time <= %v", live_info.RoomId, st, now)
		rows, err := db.Query(QuerySql)
		if err != nil {
			log.Error("从弹幕数据库读取房间状态失败。", err)
		} else {
			//延迟关闭rows
			defer rows.Close()
			a := 0
			b, c, d := 0.0, 0.0, 0.0
			if rows != nil {
				for rows.Next() {
					dm := Danmaku{}
					err := rows.Scan(&dm.msg, &dm.price)
					if err != nil {
						continue
					}
					if strings.HasPrefix(dm.msg, "投喂 ") {
						b += dm.price
					} else if strings.HasPrefix(dm.msg, "赠送 ") {
						c += dm.price
					} else if dm.price > 0 {
						d += dm.price
					} else {
						a += 1
					}
				}
			}
			stop_stmt.Exec(now, a, b, c, d, live_info.RoomId, st)
		}
	}
}
