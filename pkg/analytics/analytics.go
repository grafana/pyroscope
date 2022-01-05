/*
Package analytics deals with collecting pyroscope server usage data.

By default pyroscope server sends anonymized usage data to Pyroscope team.
This helps us understand how people use Pyroscope and prioritize features accordingly.
We take privacy of our users very seriously and only collect high-level stats
such as number of apps added, types of spies used, etc.

You can disable this with a flag or an environment variable

	pyroscope server -analytics-opt-out
	...
	PYROSCOPE_ANALYTICS_OPT_OUT=true pyroscope server

*/
package analytics

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"runtime"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

var (
	url               = "https://analytics.pyroscope.io/api/events"
	gracePeriod       = 5 * time.Minute
	uploadFrequency   = 24 * time.Hour
	snapshotFrequency = 10 * time.Minute
)

type StatsProvider interface {
	Stats() map[string]int
	AppsCount() int
}

func NewService(cfg *config.Server, s *storage.Storage, p StatsProvider) *Service {
	return &Service{
		cfg:  cfg,
		s:    s,
		p:    p,
		base: &storage.Analytics{},
		httpClient: &http.Client{
			Transport: &http.Transport{
				MaxConnsPerHost: 1,
			},
			Timeout: 60 * time.Second,
		},
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
}

type Service struct {
	cfg        *config.Server
	s          *storage.Storage
	p          StatsProvider
	base       *storage.Analytics
	httpClient *http.Client
	uploads    int

	stop chan struct{}
	done chan struct{}
}

func (s *Service) Start() {
	defer close(s.done)
	b, e := s.s.LoadAnalytics()
	if e == nil {
		s.base = b
	}

	timer := time.NewTimer(gracePeriod)
	select {
	case <-s.stop:
		return
	case <-timer.C:
	}
	s.sendReport()
	upload := time.NewTicker(uploadFrequency)
	snapshot := time.NewTicker(snapshotFrequency)
	defer upload.Stop()
	defer snapshot.Stop()
	for {
		select {
		case <-upload.C:
			s.sendReport()
		case <-snapshot.C:
			s.s.SaveAnalytics(s.getAnalytics())
		case <-s.stop:
			return
		}
	}
}

func (*Service) rebaseAnalytics(base *storage.Analytics, current *storage.Analytics) *storage.Analytics {
	rebased := &storage.Analytics{}
	vRebased := reflect.ValueOf(rebased).Elem()
	vCur := reflect.ValueOf(*current)
	vBase := reflect.ValueOf(*base)
	tAnalytics := reflect.TypeOf(*base)
	for i := 0; i < vBase.NumField(); i++ {
		name := tAnalytics.Field(i).Name
		tField := tAnalytics.Field(i).Type
		vBaseField := vBase.FieldByName(name)
		vCurrentField := vCur.FieldByName(name)
		vRebasedField := vRebased.FieldByName(name)
		tag, ok := tAnalytics.Field(i).Tag.Lookup("kind")
		if ok && tag == "cumulative" {
			switch tField.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				vRebasedField.SetInt(vBaseField.Int() + vCurrentField.Int())
			}
		}
	}
	return rebased
}

func (s *Service) Stop() {
	s.s.SaveAnalytics(s.getAnalytics())
	close(s.stop)
	<-s.done
}

func (s *Service) getAnalytics() *storage.Analytics {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	du := s.s.DiskUsage()

	controllerStats := s.p.Stats()

	a := &storage.Analytics{
		InstallID:            s.s.InstallID(),
		RunID:                uuid.New().String(),
		Version:              build.Version,
		Timestamp:            time.Now(),
		UploadIndex:          s.uploads,
		GOOS:                 runtime.GOOS,
		GOARCH:               runtime.GOARCH,
		GoVersion:            runtime.Version(),
		MemAlloc:             int(ms.Alloc),
		MemTotalAlloc:        int(ms.TotalAlloc),
		MemSys:               int(ms.Sys),
		MemNumGC:             int(ms.NumGC),
		BadgerMain:           int(du["main"]),
		BadgerTrees:          int(du["trees"]),
		BadgerDicts:          int(du["dicts"]),
		BadgerDimensions:     int(du["dimensions"]),
		BadgerSegments:       int(du["segments"]),
		ControllerIndex:      controllerStats["index"],
		ControllerComparison: controllerStats["comparison"],
		ControllerDiff:       controllerStats["diff"],
		ControllerIngest:     controllerStats["ingest"],
		ControllerRender:     controllerStats["render"],
		SpyRbspy:             controllerStats["ingest:rbspy"],
		SpyPyspy:             controllerStats["ingest:pyspy"],
		SpyGospy:             controllerStats["ingest:gospy"],
		SpyEbpfspy:           controllerStats["ingest:ebpfspy"],
		SpyPhpspy:            controllerStats["ingest:phpspy"],
		SpyDotnetspy:         controllerStats["ingest:dotnetspy"],
		SpyJavaspy:           controllerStats["ingest:javaspy"],
		AppsCount:            s.p.AppsCount(),
	}
	a = s.rebaseAnalytics(s.base, a)
	return a
}

func (s *Service) sendReport() {
	logrus.Debug("sending analytics report")

	a := s.getAnalytics()

	buf, err := json.Marshal(a)
	if err != nil {
		logrus.WithField("err", err).Error("Error happened when preparing JSON")
		return
	}
	resp, err := s.httpClient.Post(url, "application/json", bytes.NewReader(buf))
	if err != nil {
		logrus.WithField("err", err).Error("Error happened when uploading anonymized usage data")
	}
	if resp != nil {
		_, err := io.ReadAll(resp.Body)
		if err != nil {
			logrus.WithField("err", err).Error("Error happened when uploading reading server response")
			return
		}
	}

	s.uploads++
}
