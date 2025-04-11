package fsm

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"runtime/pprof"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"github.com/klauspost/compress/zstd"
	"github.com/prometheus/client_golang/prometheus"
	"go.etcd.io/bbolt"

	"github.com/grafana/pyroscope/pkg/util/ratelimit"
)

type snapshotWriter struct {
	logger      log.Logger
	tx          *bbolt.Tx
	metrics     *metrics
	compression string
	rate        int
	written     int64
}

func (s *snapshotWriter) Persist(sink raft.SnapshotSink) (err error) {
	ctx := context.Background()
	pprof.SetGoroutineLabels(pprof.WithLabels(ctx, pprof.Labels("metastore_op", "persist")))
	defer pprof.SetGoroutineLabels(ctx)

	timer := prometheus.NewTimer(s.metrics.boltDBPersistSnapshotDuration)
	level.Debug(s.logger).Log("msg", "persisting snapshot", "sink_id", sink.ID())
	defer func() {
		if err == nil {
			level.Info(s.logger).Log("msg", "persisted snapshot", "sink_id", sink.ID(), "duration", timer.ObserveDuration())
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

	// Needed to measure the actual snapshot size written to the sink.
	w := &writeCounter{Writer: sink}
	var dst io.Writer = w

	// Optionally rate limit the snapshot writes to avoid overwhelming the disk.
	// This is useful when the snapshot is written to the same disk as the WAL.
	if s.rate > 0 {
		const minRate = 10 // 10 MB/s
		if s.rate < minRate {
			s.rate = minRate
		}
		s.rate <<= 20
		dst = ratelimit.NewWriter(dst, ratelimit.NewLimiter(float64(s.rate)))
	}

	if s.compression == "zstd" {
		// We do not want to bog down the CPU with compression: we are fine with
		// slowing down the snapshotting process, as it potentially may affect the
		// WAL writes if they are located on the same disk.
		var enc *zstd.Encoder
		if enc, err = zstd.NewWriter(dst, zstd.WithEncoderConcurrency(1)); err != nil {
			level.Error(s.logger).Log("msg", "failed to create zstd encoder", "err", err)
			return err
		}
		defer func() {
			if err = enc.Close(); err != nil {
				level.Error(s.logger).Log("msg", "zstd compression failed", "err", err)
			}
		}()
		// Wrap the writer with the encoder in the last turn:
		// we want to apply the rate limiting to the sink, not
		// the encoder.
		dst = enc
	}

	level.Info(s.logger).Log("msg", "persisting snapshot", "compression", s.compression, "rate_limit_mb", s.rate)
	if _, err = s.tx.WriteTo(dst); err != nil {
		level.Error(s.logger).Log("msg", "failed to write snapshot", "err", err)
		return err
	}

	s.metrics.boltDBPersistSnapshotSize.Set(float64(w.written))
	return nil
}

type writeCounter struct {
	io.Writer
	written int64
}

func (w *writeCounter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	w.written += int64(n)
	return n, err
}

func (s *snapshotWriter) Release() {
	if s.tx != nil {
		// This is an in-memory rollback, no error expected.
		_ = s.tx.Rollback()
	}
}

type snapshotReader struct {
	io.ReadCloser
}

var zstdMagic = []byte{0x28, 0xB5, 0x2F, 0xFD}

func newSnapshotReader(snapshot io.ReadCloser) (*snapshotReader, error) {
	b := bufio.NewReader(snapshot)
	magic, err := b.Peek(4)
	if err != nil {
		return nil, err
	}

	s := snapshotReader{ReadCloser: io.NopCloser(b)}
	if bytes.Equal(magic, zstdMagic) {
		var dec *zstd.Decoder
		if dec, err = zstd.NewReader(b); err != nil {
			return nil, err
		}
		s.ReadCloser = dec.IOReadCloser()
	}

	return &s, nil
}
