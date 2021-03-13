/*
Package analytics deals with collecting pyroscope server usage data.

By default pyroscope server sends anonymized usage data to Pyroscope team. This helps us understand how people use Pyroscope and prioritize features accordingly. We take privacy of our users very seriously and only collect high-level stats such as number of apps added, types of spies used, etc.

You can disable this with a flag or an environment variable

	pyroscope server -analytics-opt-out
	...
	PYROSCOPE_ANALYTICS_OPT_OUT=true pyroscope server

*/
package analytics

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"runtime"
	"time"

	"github.com/google/uuid"
	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/server"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/sirupsen/logrus"
)

var (
	url             = "https://analytics.pyroscope.io/api/events"
	gracePeriod     = 5 * time.Minute
	uploadFrequency = 24 * time.Hour
)

func NewService(cfg *config.Config, s *storage.Storage, c *server.Controller) *Service {
	return &Service{
		cfg: cfg,
		s:   s,
		c:   c,
		httpClient: &http.Client{
			Transport: &http.Transport{
				MaxConnsPerHost: 1,
			},
			Timeout: 60 * time.Second,
		},
		stopCh: make(chan struct{}),
	}
}

type Service struct {
	cfg        *config.Config
	s          *storage.Storage
	c          *server.Controller
	httpClient *http.Client
	uploads    int
	stopCh     chan struct{}
}

type metrics struct {
	InstallID        string    `json:"install_id"`
	RunID            string    `json:"run_id"`
	Version          string    `json:"version"`
	Timestamp        time.Time `json:"timestamp"`
	UploadIndex      int       `json:"upload_index"`
	GOOS             string    `json:"goos"`
	GOARCH           string    `json:"goarch"`
	MemAlloc         int       `json:"mem_alloc"`
	MemTotalAlloc    int       `json:"mem_total_alloc"`
	MemSys           int       `json:"mem_sys"`
	MemNumGC         int       `json:"mem_num_gc"`
	BadgerMain       int       `json:"badger_main"`
	BadgerTrees      int       `json:"badger_trees"`
	BadgerDicts      int       `json:"badger_dicts"`
	BadgerDimensions int       `json:"badger_dimensions"`
	BadgerSegments   int       `json:"badger_segments"`
	ControllerIndex  int       `json:"controller_index"`
	ControllerIngest int       `json:"controller_ingest"`
	ControllerRender int       `json:"controller_render"`
	SpyRbspy         int       `json:"spy_rbspy"`
	SpyPyspy         int       `json:"spy_pyspy"`
	SpyGospy         int       `json:"spy_gospy"`
	AppsCount        int       `json:"apps_count"`
}

func (s *Service) Start() {
	timer := time.NewTimer(gracePeriod)
	<-timer.C
	s.sendReport()
	ticker := time.NewTicker(uploadFrequency)
	for {
		select {
		case <-ticker.C:
			s.sendReport()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Service) Stop() {
	select {
	case s.stopCh <- struct{}{}:
	default:
	}
	close(s.stopCh)
}

func (s *Service) sendReport() {
	logrus.Debug("sending analytics report")
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	du := s.s.DiskUsage()

	controllerStats := s.c.Stats()

	m := metrics{
		InstallID:        s.s.InstallID(),
		RunID:            uuid.New().String(),
		Version:          build.Version,
		Timestamp:        time.Now(),
		UploadIndex:      s.uploads,
		GOOS:             runtime.GOOS,
		GOARCH:           runtime.GOARCH,
		MemAlloc:         int(ms.Alloc),
		MemTotalAlloc:    int(ms.TotalAlloc),
		MemSys:           int(ms.Sys),
		MemNumGC:         int(ms.NumGC),
		BadgerMain:       int(du["main"]),
		BadgerTrees:      int(du["trees"]),
		BadgerDicts:      int(du["dicts"]),
		BadgerDimensions: int(du["dimensions"]),
		BadgerSegments:   int(du["segments"]),
		ControllerIndex:  controllerStats["index"],
		ControllerIngest: controllerStats["ingest"],
		ControllerRender: controllerStats["render"],
		SpyRbspy:         controllerStats["ingest:rbspy"],
		SpyPyspy:         controllerStats["ingest:pyspy"],
		SpyGospy:         controllerStats["ingest:gospy"],
		AppsCount:        s.c.AppsCount(),
	}

	buf, err := json.Marshal(m)
	if err != nil {
		logrus.WithField("err", err).Error("Error happened when preparing JSON")
		return
	}
	resp, err := s.httpClient.Post(url, "application/json", bytes.NewReader(buf))
	if err != nil {
		logrus.WithField("err", err).Error("Error happened when uploading anonymized usage data")
	}
	if resp != nil {
		_, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			logrus.WithField("err", err).Error("Error happened when uploading reading server response")
			return
		}
	}

	s.uploads++
}
