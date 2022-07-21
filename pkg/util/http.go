package util

import (
	"net"
	"net/http"
	"time"

	"github.com/opentracing-contrib/go-stdlib/nethttp"
	"github.com/opentracing/opentracing-go"
)

var defaultTransport http.RoundTripper = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          200,
	MaxIdleConnsPerHost:   200,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
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
