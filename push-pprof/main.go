package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptrace"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	commonconfig "github.com/prometheus/common/config"
)

var (
	url          = flag.String("url", "http://localhost:4040", "Target URL for pushing profiles")
	timeout      = flag.Duration("timeout", 60*time.Second, "Timeout for the HTTP request")
	burstSleep   = flag.Duration("iteration-sleep", 1*time.Second, "How long to sleep between iterations")
	burstSize    = flag.Int("iteration-size", 1000, "How many requests to send in a iteration")
	keylogPath   = flag.String("keylog", "", "Path to TLS keylog file (optional, no keylog if not specified)")
	debugLogPath = flag.String("debug-log", "", "Path to debug log file (optional, no debug log if not specified)")
)

var keylogWriter io.Writer
var gl log.Logger

func main() {

	flag.Parse()

	initLogging()

	var httpClient *http.Client
	var tlsOptions []commonconfig.HTTPClientOption

	if keylogWriter != nil {
		tlsOptions = append(tlsOptions, commonconfig.WithNewTLSConfigFunc(func(ctx context.Context, config *commonconfig.TLSConfig, option ...commonconfig.TLSConfigOption) (*tls.Config, error) {
			res, _ := commonconfig.NewTLSConfigWithContext(ctx, config, option...)
			if res == nil {
				panic("")
			}
			res.KeyLogWriter = keylogWriter
			return res, nil
		}))
	}

	config := commonconfig.DefaultHTTPClientConfig
	config.EnableHTTP2 = true
	httpClient, _ = commonconfig.NewClientFromConfig(config, "push-pprof-timeout-issue", tlsOptions...)

	for {
		it := time.Now()
		iteration(httpClient, *burstSize)
		_ = level.Debug(gl).Log("n", *burstSize, "iteration_time", time.Since(it), "sleeping", *burstSleep)
		time.Sleep(*burstSleep)
	}

}

func initLogging() {
	baseLogger := log.NewLogfmtLogger(os.Stdout)
	baseLogger = log.With(baseLogger, "ts", log.DefaultTimestampUTC)
	baseLogger = level.NewFilter(baseLogger, level.AllowInfo())

	if *keylogPath != "" {
		f, err := os.OpenFile(*keylogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			panic(fmt.Sprintf("Failed to open keylog file: %v", err))
		}
		keylogWriter = f
	}

	if *debugLogPath != "" {
		f, err := os.OpenFile(*debugLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			panic(fmt.Sprintf("Failed to open debug log file: %v", err))
		}

		debugLogger := log.NewLogfmtLogger(f)
		debugLogger = log.With(debugLogger, "ts", log.DefaultTimestampUTC)
		debugLogger = level.NewFilter(debugLogger, level.AllowDebug())

		gl = log.LoggerFunc(func(keyvals ...interface{}) error {
			_ = debugLogger.Log(keyvals...)
			_ = baseLogger.Log(keyvals...)
			return nil
		})
	} else {
		gl = baseLogger
	}
}

func iteration(client *http.Client, n int) {
	wg := sync.WaitGroup{}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			oneRequest(client)
		}()
	}
	wg.Wait()
}

func oneRequest(client *http.Client) {
	requestStartTime := log.DefaultTimestampUTC()
	traceId := fmt.Sprintf("%016x", rand.Uint64())
	spanId := fmt.Sprintf("%016x", rand.Uint64())
	tl := log.With(gl, "trace_id", traceId, "request_start_time", requestStartTime)
	level.Debug(tl).Log("msg", "Sending request")

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	ct := newClientTrace(level.Debug(tl))
	ctx = httptrace.WithClientTrace(ctx, ct.trace)

	req, err := http.NewRequestWithContext(ctx, "GET", *url, nil)
	if err != nil {
		_ = level.Error(tl).Log("msg", "Failed to create request", "err", err)
		return
	}

	req.Header.Set("uber-trace-id", traceId+":"+spanId+":0:1")
	req.Header.Set("jaeger-baggage", "k1=v1,k2=v2")
	req.Header.Set("User-Agent", "Tolyan/traceId:"+traceId)

	resp, err := client.Do(req)
	tl = log.With(tl, "connection", ct.connectionInfo(), "url", *url)
	if err != nil {
		_ = level.Error(tl).Log("msg", "Request failed", "err", err)
	} else {
		io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			if strings.Contains(resp.Status, "invalid token") || strings.Contains(resp.Status, "no credentials provided") {
				tl = log.With(tl, "status", resp.Status)
				_ = level.Debug(tl).Log("msg", "Request auth failed")
			} else {
				_ = level.Error(tl).Log("msg", "Request failed", "status", resp.Status)
			}
		} else {
			_ = level.Debug(tl).Log("msg", "Request successful", "status", resp.Status)
		}
	}
}

type clientTrace struct {
	trace       *httptrace.ClientTrace
	mu          sync.Mutex
	connections httptrace.GotConnInfo
}

func newClientTrace(l log.Logger) *clientTrace {
	t := &clientTrace{}
	t.trace = &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			t.logConnection(info)
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			kvs := []any{"trace_op", "DNSDone"}
			if info.Err != nil {
				kvs = append(kvs, "err", info.Err.Error())
			}
			for _, addr := range info.Addrs {
				kvs = append(kvs, "addr", addr.String())
			}
			_ = l.Log(kvs...)
		},
	}
	return t
}

func (t *clientTrace) logConnection(info httptrace.GotConnInfo) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.connections = info
}

func (t *clientTrace) connectionInfo() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.connections.Conn == nil {
		return "---"
	}
	return t.connections.Conn.LocalAddr().String() + " -> " + t.connections.Conn.RemoteAddr().String()
}
