package main

import (
	"encoding/hex"
	"math/rand"
	"net"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream/remote"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
	"github.com/pyroscope-io/pyroscope/pkg/util/slices"
	"github.com/pyroscope-io/pyroscope/pkg/util/statsd"
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
			statsd.Increment("errors.upload-error")
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

	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if conn != nil {
		conn.Close()
	}
}

func reportMetrics(prefix, k, v string) {
	r1 := regexp.MustCompile("\\s+")
	r2 := regexp.MustCompile("\\/+")
	r3 := regexp.MustCompile("[^a-zA-Z_\\-0-9\\.]+")

	k = r1.ReplaceAllString(k, "_")
	k = r2.ReplaceAllString(k, "-")
	k = r3.ReplaceAllString(k, "")

	v = r1.ReplaceAllString(v, "_")
	v = r2.ReplaceAllString(v, "-")
	v = r3.ReplaceAllString(v, "")

	statsd.Gauge(prefix+"."+k+"---"+v, 1)
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
			reportMetrics("env", pair[0], pair[1])
		}
	}

	reportMetrics("env", "GOARCH", runtime.GOARCH)
	reportMetrics("env", "GOOS", runtime.GOOS)
}

func setupLogging() {
	logrus.SetFormatter(&logrus.TextFormatter{
		TimestampFormat: "2006-01-02T15:04:05.000000",
	})
}

func main() {
	symbolBuf = make([]byte, envInt("PROFILE_SYMBOL_LENGTH"))
	setupLogging()
	if statsdAddr := os.Getenv("PYROSCOPE_STATSD_ADDR"); statsdAddr != "" {
		statsd.Initialize(statsdAddr, "pyroscope-server")
	}

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
	reportMetrics("stats", "1-start-time", startTime.Format(timeFmt))
	wg := sync.WaitGroup{}
	wg.Add(clients)
	for i := 0; i < clients; i++ {
		go startClientThread(&wg)
	}
	wg.Wait()
	logrus.Info("done sending requests")
	reportMetrics("stats", "2-stop-time", time.Now().Format(timeFmt))
	reportMetrics("stats", "duration", time.Now().Sub(startTime).String())

	time.Sleep(5 * time.Second)
	pingScreenshotTaker()
	time.Sleep(10 * time.Second)
}
