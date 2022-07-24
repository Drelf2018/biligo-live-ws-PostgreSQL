package updater

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	VersionTag = "0.1.13.4"
	repoUrl    = "https://api.github.com/repos/Drelf2018/biligo-live-ws-PostgreSQL/releases/latest"
)

var (
	log = logrus.WithField("service", "updater")
	ctx = context.Background()
)

func StartUpdater() {
	log.Info("已启动更新检查器")
	tick := time.NewTicker(time.Hour * 24)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			log.Info("正在检查更新")
			if resp, err := checkForUpdates(); err != nil {
				log.Warnf("检查更新时出现错误: %v", err)
			} else {
				version := strings.Replace(resp.TagName, "v", "", -1)
				if version > VersionTag && !resp.Prerelease {
					log.Infof("有可用新版本: %s", version)
					log.Infof("请自行到 %v 下載。", resp.HtmlUrl)
				} else {
					log.Infof("你目前已是最新版本。")
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func checkForUpdates() (*ReleaseLatestResp, error) {
	res, err := http.Get(repoUrl)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	var resp = &ReleaseLatestResp{}
	err = json.Unmarshal(b, resp)
	return resp, err
}
