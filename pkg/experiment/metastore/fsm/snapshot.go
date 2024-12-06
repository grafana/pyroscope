package fsm

import (
	"context"
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
	level.Debug(s.logger).Log("msg", "persisting snapshot", "sink_id", sink.ID())
	defer func() {
		s.metrics.boltDBPersistSnapshotDuration.Observe(time.Since(start).Seconds())
		if err == nil {
			level.Info(s.logger).Log("msg", "persisted snapshot", "sink_id", sink.ID(), "duration", time.Since(start))
			if err = sink.Close(); err != nil {
				level.Error(s.logger).Log("msg", "failed to close sink", "err", err)
			}
			return
		}
		level.Error(s.logger).Log("msg", "failed to persist snapshot", "err", err)
		if err = sink.Cancel(); err != nil {
			level.Error(s.logger).Log("msg", "failed to cancel snapshot sink", "err", err)
		}
	}()

	level.Info(s.logger).Log("msg", "persisting snapshot")
	var n int64
	n, err = s.tx.WriteTo(sink)
	s.metrics.boltDBPersistSnapshotSize.Set(float64(n))
	if err != nil {
		level.Error(s.logger).Log("msg", "failed to write snapshot", "err", err)
	}
	return err
}

func (s *snapshot) Release() {
	if s.tx != nil {
		// This is an in-memory rollback, no error expected.
		_ = s.tx.Rollback()
	}
}
