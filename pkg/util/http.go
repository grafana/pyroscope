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
	"gopkg.in/yaml.v3"
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

// WriteYAMLResponse writes some YAML as a HTTP response.
func WriteYAMLResponse(w http.ResponseWriter, v interface{}) {
	// There is not standardised content-type for YAML, text/plain ensures the
	// YAML is displayed in the browser instead of offered as a download
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	data, err := yaml.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// We ignore errors here, because we cannot do anything about them.
	// Write will trigger sending Status code, so we cannot send a different status code afterwards.
	// Also this isn't internal error, but error communicating with client.
	_, _ = w.Write(data)
}
