package main

import (
	"C"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/eric2788/biligo-live-ws/controller/listening"
	"github.com/eric2788/biligo-live-ws/controller/subscribe"
	ws "github.com/eric2788/biligo-live-ws/controller/websocket"
	"github.com/eric2788/biligo-live-ws/services/api"
	"github.com/eric2788/biligo-live-ws/services/database"
	"github.com/eric2788/biligo-live-ws/services/updater"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

var release = flag.Bool("release", os.Getenv("GIN_MODE") == "release", "set release mode")
var port = flag.Int("port", 8080, "set the websocket port")

func main() {
	Run()
}

//export Run
func Run() {

	flag.Parse()

	log.Infof("biligo-live-ws v%v", updater.VersionTag)

	if *release {
		gin.SetMode(gin.ReleaseMode)
		log.SetLevel(log.InfoLevel)
	} else {
		log.SetLevel(log.DebugLevel)
		log.Debug("启动debug模式")
	}

	log.Info("正在初始化数据库...")
	if err := database.StartDB(); err != nil {
		log.Fatalf("初始化数据库时出现严重错误: %v", err)
	} else {
		log.Info("数据库已成功初始化。")
	}

	router := gin.New()

	if os.Getenv("NO_LISTENING_LOG") == "true" {
		router.Use(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/listening") {
				c.Next()
				return
			}
			gin.Logger()(c)
		})
	}

	if os.Getenv("RESET_LOW_LATENCY") == "true" {
		go api.ResetAllLowLatency()
	}

	router.Use(CORS())
	router.Use(ErrorHandler)

	router.GET("", Index)
	router.POST("validate", ValidateProcess)

	subscribe.Register(router.Group("subscribe"))
	ws.Register(router.Group("ws"))
	listening.Register(router.Group("listening"))

	port := fmt.Sprintf(":%d", *port)

	log.Infof("使用端口 %s\n", port)

	go updater.StartUpdater()

	if err := router.Run(port); err != nil {
		log.Fatal(err)
	}

	if err := database.CloseDB(); err != nil {
		log.Errorf("关闭数据库时错误: %v", err)
	}
}

func Index(c *gin.Context) {
	c.IndentedJSON(200, gin.H{
		"status": "working",
	})
}
