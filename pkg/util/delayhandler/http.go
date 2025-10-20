package delayhandler

import (
	"context"
	"net/http"
	"time"

	"github.com/opentracing/opentracing-go"
)

func wrapResponseWriter(w http.ResponseWriter, end time.Time) (http.ResponseWriter, *delayedResponseWriter) {
	wrapped := &delayedResponseWriter{wrapped: w, end: end}

	// check if the writer implements the Flusher interface
	flusher, ok := w.(http.Flusher)
	if ok {
		return &delayedResponseWriterWithFlush{
			delayedResponseWriter: wrapped,
			flusher:               flusher,
		}, wrapped
	}

	return wrapped, wrapped
}

type delayedResponseWriterWithFlush struct {
	*delayedResponseWriter
	flusher http.Flusher
}

func (w *delayedResponseWriterWithFlush) Flush() {
	w.flusher.Flush()
}

type delayedResponseWriter struct {
	wrapped       http.ResponseWriter
	end           time.Time
	statusWritten bool
	requestError  bool
}

func (w *delayedResponseWriter) WriteHeader(statusCode int) {
	// do not forget to write the status code to the wrapped writer
	defer w.wrapped.WriteHeader(statusCode)
	w.statusWritten = true

	// errors shouldn't be delayed
	if statusCode/100 != 2 {
		w.requestError = true
		return
	}

	delayLeft := w.end.Sub(timeNow())
	if delayLeft > 0 {
		addDelayHeader(w.wrapped.Header(), delayLeft)
	}
}

func (w *delayedResponseWriter) Header() http.Header {
	return w.wrapped.Header()
}

func (w *delayedResponseWriter) Write(p []byte) (int, error) {
	return w.wrapped.Write(p)
}

func NewHTTP(limits Limits) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := timeNow()
			ctx := r.Context()

			delay := getDelay(ctx, limits)
			var delayRw *delayedResponseWriter
			delayCtx := context.Background()
			if delay > 0 {
				var cancel context.CancelFunc
				delayCtx, cancel = context.WithCancel(delayCtx)
				defer cancel()
				ctx = WithDelayCancel(ctx, cancel)
				w, delayRw = wrapResponseWriter(w, start.Add(delay))

				// only add a span when delay is active
				var sp opentracing.Span
				sp, ctx = opentracing.StartSpanFromContext(ctx, "delayhandler.Handler")
				defer sp.Finish()
			}

			// now run the chain after me
			h.ServeHTTP(w, r.WithContext(ctx))

			// if we didn't delay, return immediately
			if delayRw == nil {
				return
			}

			// if request errored we skip the delay
			if delayRw.requestError {
				return
			}

			// The delay has been canceled down the chain.
			if delayCtx.Err() != nil {
				return
			}

			delayLeft := delayRw.end.Sub(timeNow())
			// nothing to do if we're past the end time
			if delayLeft <= 0 {
				return
			}

			// when headers are not written, we add the delay header
			if !delayRw.statusWritten {
				addDelayHeader(w.Header(), delayLeft)
			}

			// create a separate span to make the artificial delay clear
			sp, _ := opentracing.StartSpanFromContext(ctx, "delayhandler.Delay")
			sp.SetTag("delayed_by", delayLeft.String())
			defer sp.Finish()

			// wait for the delay to elapse
			<-timeAfter(delayLeft)
		})
	}
}
