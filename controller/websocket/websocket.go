package websocket

import (
	"encoding/json"
	"fmt"
	live "github.com/eric2788/biligo-live"
	"github.com/eric2788/biligo-live-ws/services/blive"
	"github.com/eric2788/biligo-live-ws/services/subscriber"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

var (
	websocketTable = sync.Map{}
	log            = logrus.WithField("controller", "websocket")
)

type WebSocket struct {
	ws *websocket.Conn
	mu sync.Mutex
}

func Register(gp *gin.RouterGroup) {
	gp.GET("", OpenWebSocket)
	gp.GET("/global", OpenGlobalWebSocket)
	go blive.SubscribedRoomTracker(handleBLiveMessage)
}

func OpenWebSocket(c *gin.Context) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	// 获取辨識 Id
	id, ok := c.GetQuery("id")

	// 沒有 id 則为 anonymous
	if !ok {
		id = "anonymous"
	}

	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		_ = c.Error(err)
		return
	}

	identifier := fmt.Sprintf("%v@%v", c.ClientIP(), id)

	// 客户端正常關閉连接
	ws.SetCloseHandler(func(code int, text string) error {
		log.Infof("已关闭对 %v 的 Websocket 连接: (%v) %v", identifier, code, text)
		HandleClose(identifier)
		return ws.WriteMessage(websocket.CloseMessage, nil)
	})

	websocketTable.Store(identifier, &WebSocket{ws: ws})

	// 先前尚未有订阅
	if _, subBefore := subscriber.Get(identifier); !subBefore {
		// 使用空值防止启动订阅过期
		subscriber.Update(identifier, []int64{})
	}

	// 中止五分钟后清除订阅記憶
	subscriber.CancelExpire(identifier)

	go func() {
		for {
			// 接收客户端關閉訊息
			if _, _, err = ws.NextReader(); err != nil {
				if err := ws.Close(); err != nil {
					log.Warnf("关闭用户 %v 的 WebSocket 时发生错误: %v", identifier, err)
				}
				return
			}
		}
	}()
}

func handleBLiveMessage(room int64, info *blive.LiveInfo, msg live.Msg) {

	raw := msg.Raw()

	// 人氣值轉換为 json string
	if reply, ok := msg.(*live.MsgHeartbeatReply); ok {
		hot := reply.GetHot()
		raw = []byte(fmt.Sprintf("{\"popularity\": %v}", hot))
	}

	var content interface{}

	if err := json.Unmarshal(raw, &content); err != nil {
		log.Warnf("序列化 原始数据内容 时出现错误: %v, 将转换为 string", err)
		content = string(raw)
	}

	bLiveData := BLiveData{
		Command:  msg.Cmd(),
		LiveInfo: info,
		Content:  content,
	}

	// 订阅用户
	for _, identifier := range subscriber.GetAllSubscribers(room) {
		if err := writeMessage(identifier, bLiveData); err != nil {
			log.Warnf("向 用户 %v 发送直播数据时出现错误: (%T)%v\n", identifier, err, err)
		}
	}

	// 短号用户
	if shortRoomId, ok := blive.ShortRoomMap.Load(room); ok {

		for _, identifier := range subscriber.GetAllSubscribers(shortRoomId.(int64)) {
			if err := writeMessage(identifier, bLiveData); err != nil {
				log.Warnf("向 用户 %v 发送直播数据时出现错误: (%T)%v\n", identifier, err, err)
			}
		}

	}

	// 全局用户
	globalWebSockets.Range(func(id, conn interface{}) bool {
		if err := writeGlobalMessage(id.(string), conn.(*WebSocket), bLiveData); err != nil {
			log.Warnf("向 用户 %v 发送直播数据时出现错误: (%T)%v\n", id, err, err)
		}
		return true
	})

}

func writeMessage(identifier string, data BLiveData) error {
	conn, ok := websocketTable.Load(identifier)

	if !ok {
		//log.Infof("用户 %v 尚未连接到WS，略过發送。\n", identifier)
		return nil
	}

	socket := conn.(*WebSocket)

	defer socket.mu.Unlock()
	socket.mu.Lock()

	con := socket.ws
	byteData, err := json.Marshal(data)

	if err != nil {
		return err
	}

	if err = con.WriteMessage(websocket.TextMessage, byteData); err != nil {
		log.Warnf("向 用户 %v 发送直播数据时出现错误: (%T)%v\n", identifier, err, err)
		log.Warnf("关闭对用户 %v 的连线。", identifier)
		_ = con.Close()
		// 客户端非正常關閉连接
		HandleClose(identifier)
	}

	return nil
}

func HandleClose(identifier string) {
	websocketTable.Delete(identifier)
	// 等待五分钟，如果五分钟后沒有重连則刪除订阅記憶
	// 由于斷線的时候已经有订阅列表，因此此方法不會检查是否有订阅列表
	subscriber.ExpireAfterWithCheck(identifier, time.After(time.Minute*5), false)
}

type BLiveData struct {
	Command  string          `json:"command"`
	LiveInfo *blive.LiveInfo `json:"live_info"`
	Content  interface{}     `json:"content"`
}
