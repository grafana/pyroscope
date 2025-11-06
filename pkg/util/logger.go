package util

import (
	"bytes"
	"context"
	"io"
	"os"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/tenant"
	"github.com/grafana/dskit/tracing"
)

// Logger is a nop global logger
var Logger = log.NewNopLogger()

// LoggerWithUserID returns a Logger that has information about the current user in
// its details.
func LoggerWithUserID(tenantID string, l log.Logger) log.Logger {
	// See note in WithContext.
	return log.With(l, "tenant", tenantID)
}

// LoggerWithUserIDs returns a Logger that has information about the current user or
// users (separated by "|") in its details.
func LoggerWithUserIDs(tenantIDs []string, l log.Logger) log.Logger {
	return log.With(l, "tenant", tenant.JoinTenantIDs(tenantIDs))
}

// LoggerWithTraceID returns a Logger that has information about the traceID in
// its details.
func LoggerWithTraceID(traceID string, l log.Logger) log.Logger {
	// See note in WithContext.
	return log.With(l, "traceID", traceID)
}

// LoggerWithContext returns a Logger that has information about the current user or users
// and trace in its details.
//
// e.g.
//
//	log = util.WithContext(ctx, log)
//	# level=error tenant=user-1|user-2 traceID=123abc msg="Could not chunk chunks" err="an error"
//	level.Error(log).Log("msg", "Could not chunk chunks", "err", err)
func LoggerWithContext(ctx context.Context, l log.Logger) log.Logger {
	// Weaveworks uses "orgs" and "orgID" to represent Cortex users,
	// even though the code-base generally uses `userID` to refer to the same thing.
	userIDs, err := tenant.TenantIDs(ctx)
	if err == nil {
		l = LoggerWithUserIDs(userIDs, l)
	}

	traceID, ok := tracing.ExtractSampledTraceID(ctx)
	if !ok {
		return l
	}

	return LoggerWithTraceID(traceID, l)
}

// WithSourceIPs returns a Logger that has information about the source IPs in
// its details.
func WithSourceIPs(sourceIPs string, l log.Logger) log.Logger {
	return log.With(l, "sourceIPs", sourceIPs)
}

// AsyncWriter is a writer that buffers writes and flushes them asynchronously
// in the order they were written. It is safe for concurrent use.
//
// If the internal queue is full, writes will block until there is space.
// Errors are ignored: it's caller responsibility to handle errors from the
// underlying writer.
type AsyncWriter struct {
	mu            sync.Mutex
	w             io.Writer
	pool          sync.Pool
	buffer        *bytes.Buffer
	flushQueue    chan *bytes.Buffer
	maxSize       int
	maxCount      int
	flushInterval time.Duration
	writes        int
	closeOnce     sync.Once
	close         chan struct{}
	done          chan error
	closed        bool
}

func NewAsyncWriter(w io.Writer, bufSize, maxBuffers, maxWrites int, flushInterval time.Duration) *AsyncWriter {
	bw := &AsyncWriter{
		w:             w,
		flushQueue:    make(chan *bytes.Buffer, maxBuffers),
		maxSize:       bufSize,
		maxCount:      maxWrites,
		flushInterval: flushInterval,
		close:         make(chan struct{}),
		done:          make(chan error),
		pool: sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(make([]byte, 0, bufSize))
			},
		},
	}
	go bw.loop()
	return bw
}

func (aw *AsyncWriter) Write(p []byte) (int, error) {
	aw.mu.Lock()
	defer aw.mu.Unlock()
	if aw.closed {
		return 0, os.ErrClosed
	}
	if aw.overflows(len(p)) {
		aw.enqueueFlush()
	}
	if aw.buffer == nil {
		aw.buffer = aw.pool.Get().(*bytes.Buffer)
		aw.buffer.Reset()
	}
	aw.writes++
	return aw.buffer.Write(p)
}

func (aw *AsyncWriter) overflows(n int) bool {
	return aw.buffer != nil && (aw.buffer.Len()+n >= aw.maxSize || aw.writes >= aw.maxCount)
}

func (aw *AsyncWriter) Close() error {
	aw.closeOnce.Do(func() {
		// Break the loop.
		close(aw.close)
		<-aw.done
		// Empty the queue.
		aw.mu.Lock()
		defer aw.mu.Unlock()
		aw.enqueueFlush()
		close(aw.flushQueue)
		for buf := range aw.flushQueue {
			aw.flushSync(buf)
		}
		aw.closed = true
	})
	return nil
}

func (aw *AsyncWriter) enqueueFlush() {
	buf := aw.buffer
	if buf == nil || buf.Len() == 0 {
		return
	}
	aw.buffer = nil
	aw.writes = 0
	select {
	case aw.flushQueue <- buf:
	default:
	}
}

func (aw *AsyncWriter) loop() {
	ticker := time.NewTicker(aw.flushInterval)
	defer func() {
		ticker.Stop()
		close(aw.done)
	}()

	for {
		select {
		case buf := <-aw.flushQueue:
			aw.flushSync(buf)

		case <-ticker.C:
			aw.mu.Lock()
			aw.enqueueFlush()
			aw.mu.Unlock()

		case <-aw.close:
			return
		}
	}
}

func (aw *AsyncWriter) flushSync(b *bytes.Buffer) {
	_, _ = aw.w.Write(b.Bytes())
	aw.pool.Put(b)
}
