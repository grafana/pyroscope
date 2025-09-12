package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptrace"
	"os"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	v1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	commonconfig "github.com/prometheus/common/config"
)

func newProfile() *profilev1.Profile {
	return &profilev1.Profile{
		Function: []*profilev1.Function{
			{
				Id:   1,
				Name: 1,
			},
			{
				Id:   2,
				Name: 2,
			},
		},
		Location: []*profilev1.Location{
			{
				Id:        1,
				MappingId: 1,
				Line: []*profilev1.Line{
					{
						FunctionId: 1,
						Line:       1,
					},
				},
			},
			{
				Id:        2,
				MappingId: 1,
				Line: []*profilev1.Line{
					{
						FunctionId: 2,
						Line:       1,
					},
				},
			},
		},
		Mapping: []*profilev1.Mapping{
			{Id: 1, Filename: 3},
		},
		StringTable: []string{
			"",
			"func_a",
			"func_b",
			"my-foo-binary",
			"cpu",
			"nanoseconds",
		},
		TimeNanos: time.Now().UnixNano(),
		SampleType: []*profilev1.ValueType{{
			Type: 4,
			Unit: 5,
		}},
		Sample: []*profilev1.Sample{
			{
				Value:      []int64{1234},
				LocationId: []uint64{1},
			},
			{
				Value:      []int64{1234},
				LocationId: []uint64{1, 2},
			},
		},
	}
}

func newRequest() *pushv1.PushRequest {
	p, _ := newProfile().MarshalVT()

	return &pushv1.PushRequest{
		Series: []*pushv1.RawProfileSeries{
			{
				Labels: []*v1.LabelPair{
					{
						Name:  "service_name",
						Value: "foo",
					},
					{
						Name:  "__name__",
						Value: "cpu",
					},
				},
				Samples: []*pushv1.RawSample{
					{
						RawProfile: p,
						ID:         "",
					},
				},
			},
		},
	}
}

var (
	url        = flag.String("url", "http://localhost:4040", "Target URL for pushing profiles")
	username   = flag.String("username", "", "Basic auth username (optional)")
	password   = flag.String("password", "", "Basic auth password (optional)")
	timeout    = flag.Duration("timeout", 60*time.Second, "Timeout for the HTTP request")
	burstSleep = flag.Duration("iteration-sleep", 20*time.Second, "How long to sleep between iterations")
	burstSize  = flag.Int("iteration-size", 1000, "How many requests to send in a iteration")
)

const keylogFile = "keylog.txt"
const debugLogFile = "debug.log.txt"

var keylogWriter io.Writer
var gl log.Logger

func main() {

	flag.Parse()
	initLogging()

	var httpClient *http.Client
	tls := commonconfig.WithNewTLSConfigFunc(func(ctx context.Context, config *commonconfig.TLSConfig, option ...commonconfig.TLSConfigOption) (*tls.Config, error) {
		res, _ := commonconfig.NewTLSConfigWithContext(ctx, config, option...)
		if res == nil {
			panic("")
		}
		res.KeyLogWriter = keylogWriter
		return res, nil
	})
	config := commonconfig.DefaultHTTPClientConfig
	config.EnableHTTP2 = true
	httpClient, _ = commonconfig.NewClientFromConfig(config, "push-pprof-timeout-issue", tls)

	for {
		it := time.Now()
		iteration(httpClient, *burstSize)
		_ = gl.Log("n", *burstSize, "iteration_time", time.Since(it), "sleeping", *burstSleep)
		time.Sleep(*burstSleep)
	}

}

func initLogging() {
	var err error
	var f *os.File

	f, err = os.OpenFile(keylogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	keylogWriter = f

	f, err = os.OpenFile(debugLogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		panic(err)
	}

	baseLogger := log.NewLogfmtLogger(os.Stdout)
	baseLogger = log.With(baseLogger, "ts", log.DefaultTimestampUTC)
	baseLogger = level.NewFilter(baseLogger, level.AllowInfo())

	debugLogger := log.NewLogfmtLogger(f)
	debugLogger = log.With(debugLogger, "ts", log.DefaultTimestampUTC)
	debugLogger = level.NewFilter(debugLogger, level.AllowDebug())

	gl = log.LoggerFunc(func(keyvals ...interface{}) error {
		_ = debugLogger.Log(keyvals...)
		_ = baseLogger.Log(keyvals...)
		return nil
	})
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
	req := connect.NewRequest(newRequest())
	if *username != "" {
		req.Header().Set("Authorization", "Basic "+basicAuth(*username, *password))
	}
	req.Header().Set("uber-trace-id", traceId+":"+spanId+":0:1")
	req.Header().Set("jaeger-baggage", "k1=v1,k2=v2")
	req.Header().Set("User-Agent", "Tolyan/traceId:"+traceId)

	connectClient := pushv1connect.NewPusherServiceClientTracingThroughURL(
		client,
		*url,
		traceId,
		"",
	)

	_, err := connectClient.Push(ctx, req)
	tl = log.With(tl, "connection", ct.connectionInfo())
	if err != nil {
		_ = level.Error(tl).Log("msg", "Request failed", "err", err)
	} else {
		_ = level.Debug(tl).Log("msg", "Request successful")
	}
}

// See 2 (end of page 4) https://www.ietf.org/rfc/rfc2617.txt
// "To receive authorization, the client sends the userid and password,
// separated by a single colon (":") character, within a base64
// encoded string in the credentials."
// It is not meant to be urlencoded.
func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
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
