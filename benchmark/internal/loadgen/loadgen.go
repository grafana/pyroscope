package loadgen

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/push"
	"github.com/pyroscope-io/pyroscope/benchmark/internal/config"
	"github.com/pyroscope-io/pyroscope/benchmark/internal/server"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
	"github.com/sirupsen/logrus"
)

// how many retries to check the pyroscope server is up
const MaxReadinessRetries = 10

type Fixtures [][]*transporttrie.Trie

var defaultDurationBetweenUploads = 10 * time.Second

type LoadGen struct {
	Config    *config.LoadGen
	Rand      *rand.Rand
	SymbolBuf []byte
	Tags      []string

	runProgressMetric prometheus.Gauge
	uploadErrors      prometheus.Counter
	successfulUploads prometheus.Counter
	pusher            GatewayPusher

	// RWMutex allows one writer or multiple readers.
	//  this fits great for the case where we want to pause writes before calling render endpoint
	//  render thread becomes a "writer", and client upload threads become "readers"
	pauseMutex sync.RWMutex

	renderAppName string
	duration      time.Duration
}

type GatewayPusher interface {
	Push() error
}
type NoopGatewayPusher struct{}

func (NoopGatewayPusher) Push() error {
	return nil
}

func Cli(cfg *config.LoadGen) error {
	r := rand.New(rand.NewSource(int64(cfg.RandSeed)))

	var pusher GatewayPusher
	if cfg.PushgatewayAddress == "" {
		logrus.Debug("no pushgateway configured")
		pusher = NoopGatewayPusher{}
	} else {
		logrus.Debug("will push metrics to ", cfg.PushgatewayAddress)
		pusher = push.New(cfg.PushgatewayAddress, cfg.ServerAddress).Gatherer(prometheus.DefaultGatherer)
	}

	l := &LoadGen{
		Config:    cfg,
		Rand:      r,
		SymbolBuf: make([]byte, cfg.ProfileSymbolLength),

		runProgressMetric: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "pyroscope",
			Subsystem: "benchmark",
			Name:      "progress",
			Help:      "",
		}),
		uploadErrors: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Subsystem: "benchmark",
			Name:      "upload_errors",
			Help:      "",
		}),
		successfulUploads: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Subsystem: "benchmark",
			Name:      "successful_uploads",
			Help:      "",
		}),
		pusher: pusher,
	}

	promauto.NewGaugeFunc(
		prometheus.GaugeOpts{
			Namespace: "pyroscope",
			Subsystem: "benchmark",
			Name:      "requests_total",
			Help:      "",
		},
		func() float64 { return float64(cfg.Apps * cfg.Requests * cfg.Clients) },
	)

	if cfg.Duration != 0 {
		cfg.Requests = int(cfg.Duration / defaultDurationBetweenUploads)
	}

	l.duration = time.Duration(l.Config.Requests) * (defaultDurationBetweenUploads)

	return l.Run(cfg)
}

func (l *LoadGen) Run(cfg *config.LoadGen) error {
	time.Sleep(5 * time.Second)
	logrus.Info("running loadgenerator with config:")
	spew.Dump(cfg)

	logrus.Info("starting pull target server")
	go server.StartServer()

	logrus.Info("checking if server is available...")
	err := waitUntilEndpointReady(cfg.ServerAddress)
	if err != nil {
		return err
	}

	logrus.Info("generating fixtures")
	fixtures := l.generateFixtures()

	l.Tags = []string{}
	for i := 0; i < l.Config.TagKeys; i++ {
		tagName := fmt.Sprintf("key%d", i)
		for j := 0; j < l.Config.TagValues; j++ {
			tagValue := fmt.Sprintf("val%d", j)
			l.Tags = append(l.Tags, fmt.Sprintf("{%s=%s}", tagName, tagValue))
		}
	}
	logrus.Debug("done generating fixtures.")

	logrus.Info("starting sending requests")
	st := time.Now()
	appNameBuf := make([]byte, 25)

	servers := []string{
		l.Config.ServerAddress,
	}

	if l.Config.ServerAddress2 != "" {
		servers = append(servers, l.Config.ServerAddress2)
	}

	ctx, cancelFn := context.WithCancel(context.Background())

	wg := sync.WaitGroup{}
	wg.Add(l.Config.Apps * l.Config.Clients * len(servers))
	for _, s := range servers {
		for i := 0; i < l.Config.Apps; i++ {
			// generate a random app name
			l.Rand.Read(appNameBuf)
			appName := hex.EncodeToString(appNameBuf)
			if i == 0 {
				l.renderAppName = appName
			}
			for j := 0; j < l.Config.Clients; j++ {
				go l.startClientThread(ctx, s, j, appName, &wg, fixtures[i])
			}
		}
	}
	doneCh := make(chan struct{})
	go l.startRenderThread(doneCh)
	if cfg.TimeLimit != 0 {
		go func() {
			time.Sleep(cfg.TimeLimit)
			cancelFn()
		}()
	}
	wg.Wait()
	logrus.Info("done sending requests, benchmark took ", time.Since(st))
	doneCh <- struct{}{}

	if l.Config.NoExitWhenDone {
		logrus.Info("waiting forever")
		time.Sleep(time.Hour * 24 * 365)
	}

	return nil
}

func (l *LoadGen) generateFixtures() Fixtures {
	var f Fixtures

	for i := 0; i < l.Config.Apps; i++ {
		f = append(f, []*transporttrie.Trie{})

		randomGen := rand.New(rand.NewSource(int64(l.Config.RandSeed + i)))
		p := l.generateProfile(randomGen)
		for j := 0; j < l.Config.Fixtures; j++ {
			f[i] = append(f[i], p)
		}
	}

	return f
}

//revive:disable:argument-limit it's benchmarking code, so lower standards are fine
func (l *LoadGen) startClientThread(ctx context.Context, serverAddress string, threadID int, appName string, wg *sync.WaitGroup, appFixtures []*transporttrie.Trie) {
	rc := remote.RemoteConfig{
		UpstreamThreads:        1,
		UpstreamAddress:        serverAddress,
		UpstreamRequestTimeout: 10 * time.Second,
	}

	r, err := remote.New(rc, logrus.New())
	if err != nil {
		panic(err)
	}
	r.Start()

	requestsCount := l.Config.Requests

	threadStartTime := time.Now().Truncate(defaultDurationBetweenUploads)
	threadStartTime = threadStartTime.Add(-1 * l.duration)

	st := threadStartTime

	var timerChan <-chan time.Time
	if l.Config.RealTime {
		timerChan = time.NewTicker(defaultDurationBetweenUploads / time.Duration(l.Config.TimeMultiplier)).C
	} else {
		ch := make(chan time.Time)
		go func() {
			for i := 0; i < requestsCount; i++ {
				ch <- time.Now()
			}
			close(ch)
		}()
		timerChan = ch
	}

Outside:
	for i := 0; i < requestsCount; i++ {
		select {
		case <-ctx.Done():
			break Outside
		case <-timerChan:
		}

		t := appFixtures[i%len(appFixtures)]

		st = st.Add(10 * time.Second)
		et := st.Add(10 * time.Second)

		l.pauseMutex.RLock()
		fullName := fmt.Sprintf("%s{pod=pod-%d}", appName, threadID)
		err := r.UploadSync(&upstream.UploadJob{
			Name:            fullName,
			StartTime:       st,
			EndTime:         et,
			SpyName:         "gospy",
			SampleRate:      100,
			Units:           "samples",
			AggregationType: "sum",
			Trie:            t,
		})
		l.pauseMutex.RUnlock()
		if err != nil {
			l.uploadErrors.Add(1)
			time.Sleep(time.Second)
		} else {
			l.successfulUploads.Add(1)
		}

		err = l.pusher.Push()
		if err != nil {
			logrus.Error(err)
		}
	}

	wg.Done()
}

func (l *LoadGen) generateProfile(randomGen *rand.Rand) *transporttrie.Trie {
	t := transporttrie.New()

	for w := 0; w < l.Config.ProfileWidth; w++ {
		symbol := []byte("root")
		for d := 0; d < 2+l.Rand.Intn(l.Config.ProfileDepth); d++ {
			randomGen.Read(l.SymbolBuf)
			symbol = append(symbol, byte(';'))
			symbol = append(symbol, []byte(hex.EncodeToString(l.SymbolBuf))...)
			if l.Rand.Intn(100) <= 20 {
				t.Insert(symbol, uint64(l.Rand.Intn(100)), true)
			}
		}

		t.Insert(symbol, uint64(l.Rand.Intn(100)), true)
	}
	return t
}

// TODO(eh-am) exponential backoff and whatnot
func waitUntilEndpointReady(url string) error {
	client := http.Client{Timeout: 10 * time.Second}
	retries := 0

	for {
		_, err := client.Get(url)

		// all good?
		if err == nil {
			return nil
		}
		if retries >= MaxReadinessRetries {
			break
		}

		time.Sleep(time.Second)
		retries++
	}

	return fmt.Errorf("maximum retries exceeded ('%d') waiting for server ('%s') to respond", retries, url)
}
