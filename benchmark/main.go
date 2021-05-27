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

	st := time.Now()
	et := st.Add(10 * time.Second)

	for i := 0; i < envInt("REQUESTS"); i++ {
		t := fixtures[0]

		r.UploadSync(&upstream.UploadJob{
			Name:            "app-name{}",
			StartTime:       st,
			EndTime:         et,
			SpyName:         "gospy",
			SampleRate:      100,
			Units:           "samples",
			AggregationType: "sum",
			Trie:            t,
		})
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

func main() {

	logrus.Info("waiting for other services to load")
	// TODO: should have some health check instead
	time.Sleep(30 * time.Second)

	if statsdAddr := os.Getenv("PYROSCOPE_STATSD_ADDR"); statsdAddr != "" {
		statsd.Initialize(statsdAddr, "pyroscope-server")
	}

	for _, e := range os.Environ() {
		pair := strings.SplitN(e, "=", 2)
		if len(pair) == 2 && !slices.StringContains(excludeEnv, pair[0]) {
			reportMetrics("env", pair[0], pair[1])
		}
	}

	reportMetrics("env", "GOARCH", runtime.GOARCH)
	reportMetrics("env", "GOOS", runtime.GOOS)

	clients := envInt("CLIENTS")
	customFormatter := new(logrus.TextFormatter)
	customFormatter.TimestampFormat = "2006-01-02T15:04:05.000000"
	logrus.SetFormatter(customFormatter)

	logrus.Info("generating fixtures")
	symbolBuf := make([]byte, envInt("PROFILE_SYMBOL_LENGTH"))
	for i := 0; i < 100; i++ {
		t := transporttrie.New()
		r := rand.New(rand.NewSource(int64(envInt("RAND_SEED"))))

		for j := 0; j < envInt("PROFILE_NODES"); j++ {
			symbol := []byte("root")
			for k := 0; k < envInt("PROFILE_DEPTH"); k++ {
				r.Read(symbolBuf)
				symbol = append(symbol, []byte(";")[0])
				symbol = append(symbol, symbolBuf...)
			}
			s := hex.EncodeToString(symbol)
			t.Insert([]byte(s), 100, true)
		}

		fixtures = append(fixtures, t)
	}
	logrus.Info("done generating fixtures")

	logrus.Info("starting sending requests")
	reportMetrics("times", "1-start-time", time.Now().Format(timeFmt))
	wg := sync.WaitGroup{}
	wg.Add(clients)
	for i := 0; i < clients; i++ {
		go startClientThread(&wg)
	}
	wg.Wait()
	logrus.Info("done sending requests")
	reportMetrics("times", "2-stop-time", time.Now().Format(timeFmt))

	time.Sleep(5 * time.Second)
	pingScreenshotTaker()
	time.Sleep(10 * time.Second)
}
