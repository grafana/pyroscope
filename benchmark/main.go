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
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
	"github.com/pyroscope-io/pyroscope/pkg/util/metrics"
	"github.com/pyroscope-io/pyroscope/pkg/util/slices"
	"github.com/sirupsen/logrus"
)

var fixtures = []*transporttrie.Trie{}

func envInt(s string) int {
	v, err := strconv.Atoi(os.Getenv(s))
	if err != nil {
		panic("Please specify env variable " + s)
	}
	return v
}

func startClientThread(wg *sync.WaitGroup) {
	rc := remote.RemoteConfig{
		UpstreamThreads:        1,
		UpstreamAddress:        "http://pyroscope:4040",
		UpstreamRequestTimeout: 10 * time.Second,
	}
	r, err := remote.New(rc, logrus.StandardLogger())
	if err != nil {
		panic(err)
	}

	threadStartTime := time.Now()

	for i := 0; i < envInt("REQUESTS"); i++ {
		t := fixtures[0]

		st := threadStartTime.Add(time.Duration(i*10) * time.Second)
		et := st.Add(10 * time.Second)
		err := r.UploadSync(&upstream.UploadJob{
			Name:            "app-name{}",
			StartTime:       st,
			EndTime:         et,
			SpyName:         "gospy",
			SampleRate:      100,
			Units:           "samples",
			AggregationType: "sum",
			Trie:            t,
		})
		if err != nil {
			metrics.Count("errors.upload-error", 1)
		}
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
	rsp.WriteHeader(200)
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

func generateProfile() *transporttrie.Trie {
	t := transporttrie.New()
	r := rand.New(rand.NewSource(int64(envInt("RAND_SEED"))))

	for w := 0; w < envInt("PROFILE_WIDTH"); w++ {
		symbol := []byte("root")
		for d := 0; d < envInt("PROFILE_DEPTH"); d++ {
			r.Read(symbolBuf)
			symbol = append(symbol, byte(';'))
			symbol = append(symbol, []byte(hex.EncodeToString(symbolBuf))...)
		}

		t.Insert(symbol, 100, true)
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
	// TODO: should have some health check instead
	time.Sleep(30 * time.Second)

	reportEnvMetrics()

	clients := envInt("CLIENTS")
	logrus.Info("generating fixtures")
	for i := 0; i < 100; i++ {
		fixtures = append(fixtures, generateProfile())
	}
	logrus.Info("done generating fixtures")

	logrus.Info("starting sending requests")
	startTime := time.Now()
	reportSummaryMetric("start-time", startTime.Format(timeFmt))
	wg := sync.WaitGroup{}
	wg.Add(clients)
	for i := 0; i < clients; i++ {
		go startClientThread(&wg)
	}
	wg.Wait()
	logrus.Info("done sending requests")
	reportSummaryMetric("stop-time", time.Now().Format(timeFmt))
	reportSummaryMetric("duration", time.Since(startTime).String())

	time.Sleep(5 * time.Second)
	pingScreenshotTaker()
	time.Sleep(10 * time.Second)
}
