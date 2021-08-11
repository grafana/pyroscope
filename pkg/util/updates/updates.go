package updates

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

var latestVersionStruct struct {
	LatestVersion string `json:"latest_version"`
}

var runOnce sync.Once

func LatestVersionJSON() string {
	b, _ := json.Marshal(latestVersionStruct)
	return string(b)
}

func updateLatestVersion() error {
	resp, err := http.Get("https://pyroscope.io/latest-version.json")
	if err != nil {
		return err
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(b, &latestVersionStruct)
	if err != nil {
		return err
	}

	return nil
}

func StartVersionUpdateLoop() {
	runOnce.Do(func() {
		go func() {
			for {
				err := updateLatestVersion()
				if err != nil {
					logrus.WithError(err).Warn("failed to get update the latest version")
				}
				time.Sleep(24 * time.Hour)
			}
		}()
	})
}
