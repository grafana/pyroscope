package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"math/rand"
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
	timeout    = flag.Duration("timeout", 10*time.Second, "Timeout for the HTTP request")
	burstSleep = flag.Duration("burst-sleep", 20*time.Second, "How long to run the burst")
	burstSize  = flag.Int("burst-size", 1000, "How many requests to send in a burst")
)
var globalLogger log.Logger

func main() {

	globalLogger = log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	globalLogger = log.With(globalLogger, "ts", log.DefaultTimestampUTC)

	flag.Parse()

	cfg := commonconfig.DefaultHTTPClientConfig
	if *username != "" {
		cfg.BasicAuth = new(commonconfig.BasicAuth)
		cfg.BasicAuth.Username = *username
		cfg.BasicAuth.Password = commonconfig.Secret(*password)
	}
	httpClient, _ := commonconfig.NewClientFromConfig(cfg, "push-pprof-timeout-issue")

	client := pushv1connect.NewPusherServiceClient(
		httpClient,
		*url,
	)

	for {
		burst(client, *burstSize)
		globalLogger.Log("msg", "Sent", "n", *burstSize, "sleeping", *burstSleep)
		time.Sleep(*burstSleep)
	}

}

func burst(client pushv1connect.PusherServiceClient, n int) {
	wg := sync.WaitGroup{}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			rl := log.With(globalLogger, "req", fmt.Sprintf("%016x", rand.Uint64()))
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), *timeout)
			defer cancel()

			ct := newClientTrace()
			ctx = httptrace.WithClientTrace(ctx, ct.trace)
			req := connect.NewRequest(newRequest())
			_, err := client.Push(ctx, req)
			if err != nil {
				_ = rl.Log("err", err)
				ct.flush(rl)
			}
		}()
	}
	wg.Wait()
}

type clientTrace struct {
	trace *httptrace.ClientTrace
	es    [][]any
	mu    sync.Mutex
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
