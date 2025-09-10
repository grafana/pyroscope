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
	timeout    = flag.Duration("timeout", 30*time.Second, "Timeout for the HTTP request")
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
	config.EnableHTTP2 = true
	httpClient, _ = commonconfig.NewClientFromConfig(config, "push-pprof-timeout-issue", tls)

	for {
		it := time.Now()
		iteration(httpClient, *burstSize)
		_ = globalLogger.Log("n", *burstSize, "iteration_time", time.Since(it), "sleeping", *burstSleep)
		time.Sleep(*burstSleep)
	}

}

func initLogging() {
	var err error
	var f *os.File

	globalLogger = log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout))

	globalLogger = log.WithPrefix(globalLogger, "ts", log.DefaultTimestampUTC)

	f, err = os.OpenFile(*tracelog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	requestTraceLogger = log.NewLogfmtLogger(log.NewSyncWriter(f))
	requestTraceLogger = log.WithPrefix(requestTraceLogger, "ts", log.DefaultTimestampUTC)

	f, err = os.OpenFile(*keylog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	keylogWriter = f

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
	traceId := fmt.Sprintf("%016x", rand.Uint64())
	connectionId := traceId
	spanId := fmt.Sprintf("%016x", rand.Uint64())
	requestLogger := func(l log.Logger) log.Logger {
		return log.With(l, "trace_id", traceId, "connection_id", connectionId)
	}
	tl := requestLogger(requestTraceLogger)
	tl.Log("msg", "Sending request")

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	ct := newClientTrace(connectionId)
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
		connectionId,
	)

	_, err := connectClient.Push(ctx, req)

	if err != nil {
		_ = log.With(requestLogger(globalLogger), ct.dumpConnection()...).Log("msg", "Request failed", "err", err)
		_ = log.With(tl, ct.dumpConnection()...).Log("msg", "Request failed", "err", err)
	} else {
		_ = log.With(tl, ct.dumpConnection()...).Log("msg", "Request successful")
	}
	ct.flush(tl)
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
	trace         *httptrace.ClientTrace
	es            [][]any
	mu            sync.Mutex
	connectionIds []string
	connections   []httptrace.GotConnInfo
}

var mu sync.Mutex
var connectionToConnectionId map[connectionInfoKey]string = make(map[connectionInfoKey]string)

type connectionInfoKey struct {
	localAddr  string
	remoteAddr string
}

func newClientTrace(connectionID string) *clientTrace {
	t := &clientTrace{
		connectionIds: []string{connectionID},
	}
	t.trace = &httptrace.ClientTrace{
		GetConn: func(hostPort string) {
			t.log(
				"trace_op", "GetConn",
				"hostPort", hostPort,
			)
		},
		GotConn: func(info httptrace.GotConnInfo) {
			k := connectionInfoKey{
				localAddr:  info.Conn.LocalAddr().String(),
				remoteAddr: info.Conn.RemoteAddr().String(),
			}

			mu.Lock()
			if cid, ok := connectionToConnectionId[k]; ok {
				t.mu.Lock()
				t.connectionIds = append(t.connectionIds, cid)
				t.mu.Unlock()
			} else {
				connectionToConnectionId[k] = connectionID
			}
			mu.Unlock()
			var remoteAddr, localAddr string
			if info.Conn != nil {
				remoteAddr = info.Conn.RemoteAddr().String()
				localAddr = info.Conn.LocalAddr().String()
			}
			t.log(
				"trace_op", "GotConn",
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
				"trace_op", "PutIdleConn",
				"err", err,
			)
		},
		GotFirstResponseByte: func() {
			t.log("trace_op", "GotFirstResponseByte")
		},
		Got100Continue: nil,
		Got1xxResponse: nil,
		DNSStart: func(info httptrace.DNSStartInfo) {
			t.log(
				"trace_op", "DNSStart",
				"Host", info.Host,
			)
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			var addrs []string
			for _, addr := range info.Addrs {
				addrs = append(addrs, addr.String())
			}
			t.log(
				"trace_op", "DNSDone",
				"Addrs", strings.Join(addrs, ","),
				"Coalesced", info.Coalesced,
				"Err", info.Err,
			)
		},
		ConnectStart: func(network, addr string) {
			t.log(
				"trace_op", "ConnectStart",
				"addr", addr,
				"network", network)
		},
		ConnectDone: func(network, addr string, err error) {
			t.log(
				"trace_op", "ConnectDone",
				"addr", addr,
				"network", network,
				"err", err,
			)
		},
		TLSHandshakeStart: func() {
			t.log("trace_op", "TLSHandshakeStart")
		},
		TLSHandshakeDone: func(state tls.ConnectionState, err error) {
			t.log(
				"trace_op", "TLSHandshakeDone",
				"Version", state.Version,
				"CipherSuite", state.CipherSuite,
				"ServerName", state.ServerName,
				"NegotiatedProtocol", state.NegotiatedProtocol,
				"err", err,
			)
		},
		WroteHeaderField: nil,
		WroteHeaders: func() {
			t.log("trace_op", "WroteHeaders")
		},
		Wait100Continue: nil,
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			t.log(
				"trace_op", "WroteRequest",
				"Err", info.Err,
			)
		},
	}
	return t
}

func (t *clientTrace) log(kvs ...any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	l := append([]any{"tt", log.DefaultTimestampUTC()}, kvs...)
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

func (t *clientTrace) dumpConnection() []interface{} {
	t.mu.Lock()
	defer t.mu.Unlock()
	res := []interface{}{}
	for _, c := range t.connections {
		res = append(res,
			"LocalAddr", c.Conn.LocalAddr().String(),
			"RemoteAddr", c.Conn.RemoteAddr().String(),
			"IdleTime", c.IdleTime.String(),
			"Reused", fmt.Sprint(c.Reused),
		)
	}
	for _, id := range t.connectionIds {
		res = append(res, "ConnectionId", id)
	}
	return res
}
