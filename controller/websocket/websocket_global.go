package websocket

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var globalWebSockets = sync.Map{}

func OpenGlobalWebSocket(c *gin.Context) {

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			if os.Getenv("RESTRICT_GLOBAL") != "" {
				return c.Query("token") == os.Getenv("RESTRICT_GLOBAL")
			}
			return true
		},
		ReadBufferSize:  64,
		WriteBufferSize: 2048,
	}

	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		_ = c.Error(err)
		return
	}

	identifier := fmt.Sprintf("%v@%v", c.ClientIP(), "global")

	// 客户端正常關閉连接
	ws.SetCloseHandler(func(code int, text string) error {
		log.Infof("已关闭对 %v 的 Websocket 连接: (%v) %v", identifier, code, text)
		globalWebSockets.Delete(identifier)
		return ws.WriteMessage(websocket.CloseMessage, nil)
	})

	globalWebSockets.Store(identifier, &WebSocket{ws: ws})

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

func writeGlobalMessage(identifier string, socket *WebSocket, data BLiveData) error {

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
		globalWebSockets.Delete(identifier)
	}

	return nil
}
