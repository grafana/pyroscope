package util

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/opentracing-contrib/go-stdlib/nethttp"
	"github.com/opentracing/opentracing-go"
	"golang.org/x/net/http2"
)

var defaultTransport http.RoundTripper = &http2.Transport{
	AllowHTTP:        true,
	ReadIdleTimeout:  30 * time.Second,
	WriteByteTimeout: 30 * time.Second,
	PingTimeout:      90 * time.Second,
	DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
		return net.Dial(network, addr)
	},
}

type RoundTripperFunc func(req *http.Request) (*http.Response, error)

func (f RoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// InstrumentedHTTPClient returns a HTTP client with tracing instrumented default RoundTripper.
func InstrumentedHTTPClient() *http.Client {
	return &http.Client{
		Transport: WrapWithInstrumentedHTTPTransport(defaultTransport),
	}
}

// WrapWithInstrumentedHTTPTransport wraps the given RoundTripper with an tracing instrumented one.
func WrapWithInstrumentedHTTPTransport(next http.RoundTripper) http.RoundTripper {
	next = &nethttp.Transport{RoundTripper: next}
	return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req, tr := nethttp.TraceRequest(opentracing.GlobalTracer(), req)
		defer tr.Finish()
		return next.RoundTrip(req)
	})
}
