package main

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
	"github.com/pyroscope-io/pyroscope/pkg/util/slices"
	"github.com/sirupsen/logrus"
)

var appsCount int
var clientsCount int
var fixturesCount int
var fixtures = [][]*transporttrie.Trie{}

func envInt(s string) int {
	v, err := strconv.Atoi(os.Getenv(s))
	if err != nil {
		panic("Please specify env variable " + s)
	}
	return v
}

var requestsCompleteCount uint64

func waitUntilEndpointReady(url string) {
	for {
		logrus.Infof("checking endpoint %s", url)

		_, err := http.Get(url)
		if err != nil {
			break
		}
		time.Sleep(time.Second)
	}
}

func startClientThread(appName string, wg *sync.WaitGroup, appFixtures []*transporttrie.Trie, runProgress prometheus.Gauge, successfulUploads prometheus.Counter, uploadErrors prometheus.Counter) {
	rc := remote.RemoteConfig{
		UpstreamThreads:        1,
		UpstreamAddress:        "http://pyroscope:4040",
		UpstreamRequestTimeout: 10 * time.Second,
	}
	r, err := remote.New(rc, logrus.StandardLogger())
	if err != nil {
		panic(err)
	}

	requestsCount := envInt("REQUESTS")

	threadStartTime := time.Now().Truncate(10 * time.Second)
	threadStartTime = threadStartTime.Add(time.Duration(-1*requestsCount) * (10 * time.Second))

	st := threadStartTime

	for i := 0; i < requestsCount; i++ {
		t := appFixtures[i%len(appFixtures)]

		st = st.Add(10 * time.Second)
		et := st.Add(10 * time.Second)
		err := r.UploadSync(&upstream.UploadJob{
			Name:            appName + "{}",
			StartTime:       st,
			EndTime:         et,
			SpyName:         "gospy",
			SampleRate:      100,
			Units:           "samples",
			AggregationType: "sum",
			Trie:            t,
		})
		if err != nil {
			uploadErrors.Add(1)
			time.Sleep(time.Second)
		} else {
			successfulUploads.Add(1)
		}
		atomic.AddUint64(&requestsCompleteCount, 1)
		runProgress.Set(float64(requestsCompleteCount) / (float64(appsCount * requestsCount * clientsCount)))
	}

	wg.Done()
}

func pingScreenshotTaker() {
	logrus.Info("taking screenshots")
	tcpAddr, err := net.ResolveTCPAddr("tcp", "host.docker.internal:30014")
	if err != nil {
		panic(err)
	}

	conn, _ := net.DialTCP("tcp", nil, tcpAddr)
	if conn != nil {
		conn.Close()
	}
}

var summaryText = `
<style>
	body {
		font-family: SFMono-Regular,Consolas,Liberation Mono,Menlo,monospace;
		font-size: 12px;
		color: #ddd;
	}
</style>
`

func printSummary(rsp http.ResponseWriter, _ *http.Request) {
	rsp.Header().Add("Content-Type", "text/html")
	rsp.Write([]byte(summaryText))
}

func reportSummaryMetric(k, v string) {
	summaryText += fmt.Sprintf("%s=%s<br>\n", k, v)
}

const timeFmt = "2006-01-02T15-04-05-000"

var excludeEnv = []string{
	"PATH",
	"GOPATH",
	"HOME",
	"PYROSCOPE_STATSD_ADDR",
}

var symbolBuf []byte

var r *rand.Rand

func init() {
	r = rand.New(rand.NewSource(int64(envInt("RAND_SEED"))))
}

func generateProfile(randomGen *rand.Rand) *transporttrie.Trie {
	t := transporttrie.New()

	for w := 0; w < envInt("PROFILE_WIDTH"); w++ {
		symbol := []byte("root")
		for d := 0; d < 2+r.Intn(envInt("PROFILE_DEPTH")); d++ {
			randomGen.Read(symbolBuf)
			symbol = append(symbol, byte(';'))
			symbol = append(symbol, []byte(hex.EncodeToString(symbolBuf))...)
			if r.Intn(100) <= 20 {
				t.Insert(symbol, uint64(r.Intn(100)), true)
			}
		}

		t.Insert(symbol, uint64(r.Intn(100)), true)
	}
	return t
}

func reportEnvMetrics() {
	for _, e := range os.Environ() {
		pair := strings.SplitN(e, "=", 2)
		if len(pair) == 2 && !slices.StringContains(excludeEnv, pair[0]) {
			reportSummaryMetric(pair[0], pair[1])
		}
	}

	reportSummaryMetric("GOARCH", runtime.GOARCH)
	reportSummaryMetric("GOOS", runtime.GOOS)
}

func setupLogging() {
	logrus.SetLevel(logrus.InfoLevel)
	logrus.SetFormatter(&logrus.TextFormatter{
		TimestampFormat: "2006-01-02T15:04:05.000000",
	})
}

func main() {
	symbolBuf = make([]byte, envInt("PROFILE_SYMBOL_LENGTH"))
	setupLogging()

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/summary", printSummary)
	go http.ListenAndServe(":8081", nil)

	logrus.Info("waiting for other services to load")

	runProgress := promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "pyroscope",
		Subsystem: "benchmark",
		Name:      "progress",
		Help:      "",
	})

	uploadErrors := promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "pyroscope",
		Subsystem: "benchmark",
		Name:      "upload_errors",
		Help:      "",
	})
	successfulUploads := promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "pyroscope",
		Subsystem: "benchmark",
		Name:      "successful_uploads",
		Help:      "",
	})

	waitUntilEndpointReady("pyroscope:4040")
	waitUntilEndpointReady("prometheus:9090")

	reportEnvMetrics()

	appsCount = envInt("APPS")
	clientsCount = envInt("CLIENTS")
	fixturesCount = envInt("FIXTURES")

	logrus.Info("generating fixtures")
	for i := 0; i < appsCount; i++ {
		fixtures = append(fixtures, []*transporttrie.Trie{})

		randomGen := rand.New(rand.NewSource(int64(envInt("RAND_SEED") + i)))
		p := generateProfile(randomGen)
		for j := 0; j < fixturesCount; j++ {
			fixtures[i] = append(fixtures[i], p)
		}
	}

	logrus.Info("done generating fixtures")

	logrus.Info("starting sending requests")
	startTime := time.Now()
	reportSummaryMetric("start-time", startTime.Format(timeFmt))
	wg := sync.WaitGroup{}
	wg.Add(appsCount * clientsCount)

	appNameBuf := make([]byte, 25)
	for i := 0; i < appsCount; i++ {
		r.Read(appNameBuf)
		for j := 0; j < clientsCount; j++ {
			go startClientThread(hex.EncodeToString(appNameBuf), &wg, fixtures[i], runProgress, successfulUploads, uploadErrors)
		}
	}
	wg.Wait()
	logrus.Info("done sending requests")
	reportSummaryMetric("stop-time", time.Now().Format(timeFmt))
	reportSummaryMetric("duration", time.Since(startTime).String())

	time.Sleep(5 * time.Second)
	pingScreenshotTaker()
	time.Sleep(10 * time.Second)
	if os.Getenv("WAIT") != "" {
		select {}
	}
}
