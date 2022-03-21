package analytics

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/sirupsen/logrus"
)

var (
	adhocURL        = "https://adhoc.analytics.pyroscope.io/api/adhoc-events"
	adhocHTTPClient *http.Client
)

func init() {
	adhocHTTPClient = &http.Client{
		Transport: &http.Transport{
			MaxConnsPerHost: 1,
		},
		Timeout: 10 * time.Second,
	}
}

type AdhocEvent struct {
	Version   string    `json:"version"`
	GitSHA    string    `json:"git_sha"`
	BuildTime string    `json:"build_time"`
	Timestamp time.Time `json:"timestamp"`
	GOOS      string    `json:"goos"`
	GOARCH    string    `json:"goarch"`
	GoVersion string    `json:"go_version"`

	EventName string `json:"event_name"`
}

func AdhocReport(eventName string, wg *sync.WaitGroup) {
	defer wg.Done()

	logrus.Debug("sending adhoc analytics report")

	ev := &AdhocEvent{
		Version:   build.Version,
		GitSHA:    build.GitSHA,
		BuildTime: build.Time,
		Timestamp: time.Now(),
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
		GoVersion: runtime.Version(),

		EventName: eventName,
	}

	buf, err := json.Marshal(ev)
	if err != nil {
		logrus.WithField("err", err).Debug("Error happened when preparing JSON")
		return
	}
	resp, err := adhocHTTPClient.Post(adhocURL, "application/json", bytes.NewReader(buf))
	if err != nil {
		logrus.WithField("err", err).Debug("Error happened when uploading anonymized usage data")
	}
	if resp != nil {
		_, err := io.ReadAll(resp.Body)
		if err != nil {
			logrus.WithField("err", err).Debug("Error happened when uploading reading server response")
			return
		}
	}
}
