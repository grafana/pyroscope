package nethttp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptrace"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
)

type contextKey int

const (
	keyTracer     contextKey = iota
	componentName            = "pyroscope/net/http"
)

type Transport struct {
	// The actual RoundTripper to use for the request. A nil RoundTripper defaults to http.DefaultTransport.
	http.RoundTripper
}

func TraceRequest(tr opentracing.Tracer, req *http.Request) *http.Request {
	ht := &Tracer{tr: tr}
	ctx := req.Context()
	ctx = httptrace.WithClientTrace(ctx, ht.clientTrace())
	req = req.WithContext(context.WithValue(ctx, keyTracer, ht))
	return req
}

type closeTracker struct {
	io.ReadCloser
	sp opentracing.Span
}

func (c closeTracker) Close() error {
	err := c.ReadCloser.Close()
	c.sp.LogFields(log.String("event", "ClosedBody"))
	c.sp.Finish()
	return err
}

func TracerFromRequest(req *http.Request) *Tracer {
	tr, ok := req.Context().Value(keyTracer).(*Tracer)
	if !ok {
		return nil
	}
	return tr
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt := t.RoundTripper
	if rt == nil {
		rt = http.DefaultTransport
	}
	tracer := TracerFromRequest(req)
	if tracer == nil {
		return rt.RoundTrip(req)
	}

	tracer.start(req)

	carrier := opentracing.HTTPHeadersCarrier(req.Header)
	err := tracer.sp.Tracer().Inject(tracer.sp.Context(), opentracing.HTTPHeaders, carrier)
	if err != nil {
		return rt.RoundTrip(req)
	}

	resp, err := rt.RoundTrip(req)

	if err != nil {
		tracer.sp.Finish()
		return resp, err
	}
	ext.HTTPStatusCode.Set(tracer.sp, uint16(resp.StatusCode))
	if resp.StatusCode >= http.StatusInternalServerError {
		ext.Error.Set(tracer.sp, true)
	}
	// Normally the span is finished when the response body is closed, but with streaming the initial HTTP response
	// does not have a body and this never happens. We are patching this here by finishing the span early, knowing
	// that this will make the span shorter than what it actually is.
	if req.Method == "HEAD" || resp.ContentLength < 0 {
		tracer.sp.Finish()
	} else {
		resp.Body = closeTracker{resp.Body, tracer.sp}
	}
	return resp, nil
}

type Tracer struct {
	tr opentracing.Tracer
	sp opentracing.Span
}

func (t *Tracer) start(req *http.Request) opentracing.Span {
	ctx := opentracing.SpanFromContext(req.Context()).Context()
	t.sp = t.tr.StartSpan("HTTP "+req.Method, opentracing.ChildOf(ctx))
	ext.SpanKindRPCClient.Set(t.sp)
	ext.Component.Set(t.sp, componentName)
	ext.HTTPMethod.Set(t.sp, req.Method)
	ext.HTTPUrl.Set(t.sp, req.URL.String())
	return t.sp
}

func (t *Tracer) clientTrace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		GetConn:              t.getConn,
		GotConn:              t.gotConn,
		PutIdleConn:          t.putIdleConn,
		GotFirstResponseByte: t.gotFirstResponseByte,
		Got100Continue:       t.got100Continue,
		DNSStart:             t.dnsStart,
		DNSDone:              t.dnsDone,
		ConnectStart:         t.connectStart,
		ConnectDone:          t.connectDone,
		WroteHeaders:         t.wroteHeaders,
		Wait100Continue:      t.wait100Continue,
		WroteRequest:         t.wroteRequest,
	}
}

func (t *Tracer) getConn(hostPort string) {
	ext.HTTPUrl.Set(t.sp, hostPort)
	t.sp.LogFields(log.String("event", "GetConn"))
}

func (t *Tracer) gotConn(info httptrace.GotConnInfo) {
	t.sp.SetTag("net/http.reused", info.Reused)
	t.sp.SetTag("net/http.was_idle", info.WasIdle)
	t.sp.LogFields(log.String("event", "GotConn"))
}

func (t *Tracer) putIdleConn(error) {
	t.sp.LogFields(log.String("event", "PutIdleConn"))
}

func (t *Tracer) gotFirstResponseByte() {
	t.sp.LogFields(log.String("event", "GotFirstResponseByte"))
}

func (t *Tracer) got100Continue() {
	t.sp.LogFields(log.String("event", "Got100Continue"))
}

func (t *Tracer) dnsStart(info httptrace.DNSStartInfo) {
	t.sp.LogFields(
		log.String("event", "DNSStart"),
		log.String("host", info.Host),
	)
}

func (t *Tracer) dnsDone(info httptrace.DNSDoneInfo) {
	fields := []log.Field{log.String("event", "DNSDone")}
	for _, addr := range info.Addrs {
		fields = append(fields, log.String("addr", addr.String()))
	}
	if info.Err != nil {
		fields = append(fields, log.Error(info.Err))
	}
	t.sp.LogFields(fields...)
}

func (t *Tracer) connectStart(network, addr string) {
	t.sp.LogFields(
		log.String("event", "ConnectStart"),
		log.String("network", network),
		log.String("addr", addr),
	)
}

func (t *Tracer) connectDone(network, addr string, err error) {
	if err != nil {
		t.sp.LogFields(
			log.String("message", "ConnectDone"),
			log.String("network", network),
			log.String("addr", addr),
			log.String("event", "error"),
			log.Error(err),
		)
	} else {
		t.sp.LogFields(
			log.String("event", "ConnectDone"),
			log.String("network", network),
			log.String("addr", addr),
		)
	}
}

func (t *Tracer) wroteHeaders() {
	t.sp.LogFields(log.String("event", "WroteHeaders"))
}

func (t *Tracer) wait100Continue() {
	t.sp.LogFields(log.String("event", "Wait100Continue"))
}

func (t *Tracer) wroteRequest(info httptrace.WroteRequestInfo) {
	if info.Err != nil {
		t.sp.LogFields(
			log.String("message", "WroteRequest"),
			log.String("event", "error"),
			log.Error(info.Err),
		)
		ext.Error.Set(t.sp, true)
	} else {
		t.sp.LogFields(log.String("event", "WroteRequest"))
	}
}
