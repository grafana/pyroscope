package http

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/baggage"
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

type RoundTripperInstrumentFunc func(next http.RoundTripper) http.RoundTripper

// InstrumentedDefaultHTTPClient returns an http client configured with some
// default settings which is wrapped with a variety of instrumented
// RoundTrippers.
func InstrumentedDefaultHTTPClient(instruments ...RoundTripperInstrumentFunc) *http.Client {
	client := &http.Client{
		Transport: defaultTransport,
	}
	return InstrumentedHTTPClient(client, instruments...)
}

// InstrumentedHTTPClient adds the associated instrumentation middlewares to the
// provided http client.
func InstrumentedHTTPClient(client *http.Client, instruments ...RoundTripperInstrumentFunc) *http.Client {
	for i := len(instruments) - 1; i >= 0; i-- {
		client.Transport = instruments[i](client.Transport)
	}
	return client
}

// WithTracingTransport wraps the given RoundTripper with a tracing instrumented
// one.
func WithTracingTransport() RoundTripperInstrumentFunc {
	return func(next http.RoundTripper) http.RoundTripper {
		return otelhttp.NewTransport(next)
	}
}

// WithBaggageTransport will set the Baggage header on the request if there is
// any baggage in the context and it was not already set.
func WithBaggageTransport() RoundTripperInstrumentFunc {
	return func(next http.RoundTripper) http.RoundTripper {
		return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			_, ok := req.Header["Baggage"]
			if ok {
				return next.RoundTrip(req)
			}

			b := baggage.FromContext(req.Context())
			if b.Len() == 0 {
				return next.RoundTrip(req)
			}

			req.Header.Set("Baggage", b.String())
			return next.RoundTrip(req)
		})
	}
}
