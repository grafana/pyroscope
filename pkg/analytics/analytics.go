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

type Analytics struct {
	// metadata
	InstallID            string    `json:"install_id"`
	RunID                string    `json:"run_id"`
	Version              string    `json:"version"`
	GitSHA               string    `json:"git_sha"`
	BuildTime            string    `json:"build_time"`
	Timestamp            time.Time `json:"timestamp"`
	UploadIndex          int       `json:"upload_index"`
	GOOS                 string    `json:"goos"`
	GOARCH               string    `json:"goarch"`
	GoVersion            string    `json:"go_version"`
	AnalyticsPersistence bool      `json:"analytics_persistence"`

	// gauges
	MemAlloc         int `json:"mem_alloc"`
	MemTotalAlloc    int `json:"mem_total_alloc"`
	MemSys           int `json:"mem_sys"`
	MemNumGC         int `json:"mem_num_gc"`
	BadgerMain       int `json:"badger_main"`
	BadgerTrees      int `json:"badger_trees"`
	BadgerDicts      int `json:"badger_dicts"`
	BadgerDimensions int `json:"badger_dimensions"`
	BadgerSegments   int `json:"badger_segments"`
	AppsCount        int `json:"apps_count"`

	// counters
	ControllerIndex      int `json:"controller_index" kind:"cumulative"`
	ControllerComparison int `json:"controller_comparison" kind:"cumulative"`
	ControllerDiff       int `json:"controller_diff" kind:"cumulative"`
	ControllerIngest     int `json:"controller_ingest" kind:"cumulative"`
	ControllerRender     int `json:"controller_render" kind:"cumulative"`
	SpyRbspy             int `json:"spy_rbspy" kind:"cumulative"`
	SpyPyspy             int `json:"spy_pyspy" kind:"cumulative"`
	SpyGospy             int `json:"spy_gospy" kind:"cumulative"`
	SpyEbpfspy           int `json:"spy_ebpfspy" kind:"cumulative"`
	SpyPhpspy            int `json:"spy_phpspy" kind:"cumulative"`
	SpyDotnetspy         int `json:"spy_dotnetspy" kind:"cumulative"`
	SpyJavaspy           int `json:"spy_javaspy" kind:"cumulative"`
}

type StatsProvider interface {
	Stats() map[string]int
	AppsCount() int
}

func NewService(cfg *config.Server, s *storage.Storage, p StatsProvider) *Service {
	return &Service{
		cfg:  cfg,
		s:    s,
		p:    p,
		base: &Analytics{},
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
	base       *Analytics
	httpClient *http.Client
	uploads    int

	stop chan struct{}
	done chan struct{}
}

func (s *Service) Start() {
	defer close(s.done)
	err := s.s.LoadAnalytics(s.base)
	if err != nil {
		logrus.WithError(err).Error("failed to load analytics data")
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

// TODO: reflection is always tricky to work with. Maybe long term we should just put all counters
//   in one map (map[string]int), and put all gauges in another map(map[string]int) and then
//   for gauges we would override old values and for counters we would sum the values up.
func (*Service) rebaseAnalytics(base *Analytics, current *Analytics) *Analytics {
	rebased := &Analytics{}
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

func (s *Service) getAnalytics() *Analytics {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	du := s.s.DiskUsage()

	controllerStats := s.p.Stats()

	a := &Analytics{
		// metadata
		InstallID:            s.s.InstallID(),
		RunID:                uuid.New().String(),
		Version:              build.Version,
		GitSHA:               build.GitSHA,
		BuildTime:            build.Time,
		Timestamp:            time.Now(),
		UploadIndex:          s.uploads,
		GOOS:                 runtime.GOOS,
		GOARCH:               runtime.GOARCH,
		GoVersion:            runtime.Version(),
		AnalyticsPersistence: true,

		// gauges
		MemAlloc:         int(ms.Alloc),
		MemTotalAlloc:    int(ms.TotalAlloc),
		MemSys:           int(ms.Sys),
		MemNumGC:         int(ms.NumGC),
		BadgerMain:       int(du["main"]),
		BadgerTrees:      int(du["trees"]),
		BadgerDicts:      int(du["dicts"]),
		BadgerDimensions: int(du["dimensions"]),
		BadgerSegments:   int(du["segments"]),
		AppsCount:        s.p.AppsCount(),

		// counters
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
