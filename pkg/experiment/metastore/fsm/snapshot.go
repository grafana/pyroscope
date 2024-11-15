package fsm

import (
	"context"
	"io"
	"runtime/pprof"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"
)

type snapshot struct {
	logger  log.Logger
	tx      *bbolt.Tx
	metrics *metrics
}

func (s *snapshot) Persist(sink raft.SnapshotSink) (err error) {
	ctx := context.Background()
	pprof.SetGoroutineLabels(pprof.WithLabels(ctx, pprof.Labels("metastore_op", "persist")))
	defer pprof.SetGoroutineLabels(ctx)

	start := time.Now()
	_ = s.logger.Log("msg", "persisting snapshot", "sink_id", sink.ID())
	defer func() {
		s.metrics.boltDBPersistSnapshotDuration.Observe(time.Since(start).Seconds())
		s.logger.Log("msg", "persisted snapshot", "sink_id", sink.ID(), "err", err, "duration", time.Since(start))
		if err != nil {
			_ = s.logger.Log("msg", "failed to persist snapshot", "err", err)
			if err = sink.Cancel(); err != nil {
				_ = s.logger.Log("msg", "failed to cancel snapshot sink", "err", err)
				return
			}
		}
		if err = sink.Close(); err != nil {
			_ = s.logger.Log("msg", "failed to close sink", "err", err)
		}
	}()

	_ = level.Info(s.logger).Log("msg", "persisting snapshot")
	w := newSnapshotWriter(sink)
	_, err = s.tx.WriteTo(w)
	s.metrics.boltDBPersistSnapshotSize.Observe(float64(w.size))
	if err != nil {
		_ = level.Error(s.logger).Log("msg", "failed to write snapshot", "err", err)
	}
	return err
}

func (s *snapshot) Release() {
	if s.tx != nil {
		// This is an in-memory rollback, no error expected.
		_ = s.tx.Rollback()
	}
}

type snapshotWriter struct {
	io.Writer
	size int
}

func newSnapshotWriter(w io.Writer) *snapshotWriter { return &snapshotWriter{Writer: w} }

func (s *snapshotWriter) Write(p []byte) (int, error) {
	s.size += len(p)
	n, err := s.Writer.Write(p)
	return n, err
}
