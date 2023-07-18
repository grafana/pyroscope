package loadgen

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

var defaultRenderInterval = 1 * time.Minute

func (l *LoadGen) startRenderThread(done chan struct{}) {
	ticker := time.NewTicker(defaultRenderInterval)
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			logrus.Info("calling render endpoint under load")
			l.callRenderEndpoint()

			logrus.Info("calling render endpoint without load")
			l.pauseMutex.Lock()
			l.callRenderEndpoint()
			l.pauseMutex.Unlock()
		}
	}
}

func (l *LoadGen) callRenderEndpoint() {
	url := fmt.Sprintf(
		"%s/render?from=now-%dh&until=now&query=%s%%7B%%7D&max-nodes=1024&format=json",
		l.Config.ServerAddress,
		l.duration/time.Hour,
		l.renderAppName,
	)
	for i := 0; i < 1; i++ {
		st := time.Now()
		client := http.Client{
			Timeout: 360 * time.Second,
		}
		req, err := client.Get(url)
		// req, err := http.Get(url)
		if req != nil && req.Body != nil && err == nil {
			b, err := ioutil.ReadAll(req.Body)
			l := 100
			if len(b) < l {
				l = len(b)
			}
			logrus.Debug("body", string(b[:l]), err)
		}
		logrus.Infof("render req %d time %q %q", i, time.Now().Sub(st), err)
	}
}
