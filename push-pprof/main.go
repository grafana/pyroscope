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
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
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
	keylog     = flag.String("keylog", "keylog.txt", "")
	tracelog   = flag.String("tracelog", "tracelog.txt", "")
	timeout    = flag.Duration("timeout", 10*time.Second, "Timeout for the HTTP request")
	burstSleep = flag.Duration("iteration-sleep", 20*time.Second, "How long to sleep between iterations")
	burstSize  = flag.Int("iteration-size", 1000, "How many requests to send in a iteration")
)
var globalLogger log.Logger
var requestTraceLogger log.Logger
var keylogWriter io.Writer

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
	config.EnableHTTP2 = false
	httpClient, _ = commonconfig.NewClientFromConfig(config, "push-pprof-timeout-issue", tls)

	client := pushv1connect.NewPusherServiceClient(
		httpClient,
		*url,
	)

	for {
		it := time.Now()
		iteration(client, *burstSize)
		_ = globalLogger.Log("msg", "Sent", "n", *burstSize, "iteration-time", time.Since(it), "sleeping", *burstSleep)
		time.Sleep(*burstSleep)
	}

}

func initLogging() {
	var err error
	var f *os.File

	st := time.Now()
	globalLogger = log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout))
	globalLogger = log.WithSuffix(globalLogger,
		"tt", log.Valuer(func() interface{} {
			return time.Since(st)
		}))
	globalLogger = log.WithPrefix(globalLogger, "ts", log.DefaultTimestampUTC)

	f, err = os.OpenFile(*tracelog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	requestTraceLogger = log.NewLogfmtLogger(log.NewSyncWriter(f))
	requestTraceLogger = log.WithPrefix(requestTraceLogger, "ts", log.DefaultTimestampUTC)
	requestTraceLogger = log.With(requestTraceLogger, "tt", log.Valuer(func() interface{} {
		return time.Since(st)
	}))

	f, err = os.OpenFile(*keylog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	keylogWriter = f

}

func iteration(client pushv1connect.PusherServiceClient, n int) {
	wg := sync.WaitGroup{}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			traceId := fmt.Sprintf("%016x", rand.Uint64())

			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), *timeout)
			defer cancel()

			ct := newClientTrace()
			ctx = httptrace.WithClientTrace(ctx, ct.trace)
			req := connect.NewRequest(newRequest())
			if *username != "" {
				req.Header().Set("Authorization", "Basic "+basicAuth(*username, *password))
			}
			req.Header().Set("uber-trace-id", traceId+":0:0:1")
			req.Header().Set("jaeger-baggage", "k1=v1,k2=v2")
			req.Header().Set("User-Agent", "Tolyan/"+traceId+":0:0:1")
			_, err := client.Push(ctx, req)
			requstLogger := func(l log.Logger, err error) log.Logger {
				var kv []interface{}
				kv = append(kv, "traceId", traceId)
				if err != nil {
					kv = append(kv, "err", err)
				}
				for _, s := range ct.dumpConnection() {
					kv = append(kv, "con", s)
				}
				return log.With(l, kv...)
			}

			tl := requstLogger(requestTraceLogger, err)
			_ = tl.Log()
			ct.flush(tl)
			if err != nil {
				_ = requstLogger(globalLogger, err).Log()
			}
		}()
	}
	wg.Wait()
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
	trace *httptrace.ClientTrace
	es    [][]any
	mu    sync.Mutex

	connections []httptrace.GotConnInfo
}

func newClientTrace() *clientTrace {
	t := &clientTrace{}
	t.trace = &httptrace.ClientTrace{
		GetConn: func(hostPort string) {
			t.log(
				"msg", "GetConn",
				"hostPort", hostPort,
			)
		},
		GotConn: func(info httptrace.GotConnInfo) {
			var remoteAddr, localAddr string
			if info.Conn != nil {
				remoteAddr = info.Conn.RemoteAddr().String()
				localAddr = info.Conn.LocalAddr().String()
			}
			t.log(
				"msg", "GotConn",
				"Reused", info.Reused,
				"WasIdle", info.WasIdle,
				"IdleTime", info.IdleTime,
				"RemoteAddr", remoteAddr,
				"LocalAddr", localAddr,
			)
			t.logConnection(info)
		},
		PutIdleConn: func(err error) {
			t.log(
				"msg", "PutIdleConn",
				"err", err,
			)
		},
		GotFirstResponseByte: func() {
			t.log("msg", "GotFirstResponseByte")
		},
		Got100Continue: nil,
		Got1xxResponse: nil,
		DNSStart: func(info httptrace.DNSStartInfo) {
			t.log(
				"msg", "DNSStart",
				"Host", info.Host,
			)
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			var addrs []string
			for _, addr := range info.Addrs {
				addrs = append(addrs, addr.String())
			}
			t.log(
				"msg", "DNSDone",
				"Addrs", strings.Join(addrs, ","),
				"Coalesced", info.Coalesced,
				"Err", info.Err,
			)
		},
		ConnectStart: func(network, addr string) {
			t.log(
				"msg", "ConnectStart",
				"addr", addr,
				"network", network)
		},
		ConnectDone: func(network, addr string, err error) {
			t.log(
				"msg", "ConnectDone",
				"addr", addr,
				"network", network,
				"err", err,
			)
		},
		TLSHandshakeStart: func() {
			t.log("msg", "TLSHandshakeStart")
		},
		TLSHandshakeDone: func(state tls.ConnectionState, err error) {
			t.log(
				"msg", "TLSHandshakeDone",
				"Version", state.Version,
				"CipherSuite", state.CipherSuite,
				"ServerName", state.ServerName,
				"NegotiatedProtocol", state.NegotiatedProtocol,
				"err", err,
			)
		},
		WroteHeaderField: nil,
		WroteHeaders: func() {
			t.log("msg", "WroteHeaders")
		},
		Wait100Continue: nil,
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			t.log(
				"msg", "WroteRequest",
				"Err", info.Err,
			)
		},
	}
	return t
}

func (t *clientTrace) log(kvs ...any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	l := append([]any{"tt", time.Now()}, kvs...)
	t.es = append(t.es, l)
}

func (t *clientTrace) flush(logger log.Logger) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, e := range t.es {
		_ = logger.Log(e...)
	}
}

func (t *clientTrace) logConnection(info httptrace.GotConnInfo) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.connections = append(t.connections, info)
}

func (t *clientTrace) dumpConnection() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	res := []string{}
	for _, c := range t.connections {

		s := fmt.Sprintf("Connection{LocalAddr = %s RemoteAddr = %s  IdleTime = %s Reused = %v }\n",
			c.Conn.LocalAddr().String(), c.Conn.RemoteAddr().String(), c.IdleTime, c.Reused)
		res = append(res, s)
	}
	return res
}
