package fsm

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

// newTestFSM creates a minimal FSM wired to a real BoltDB in a temp directory.
// The background compactor is NOT started automatically; tests control it
// manually via fsm.compactor.
func newTestFSM(t *testing.T, cfg Config) *FSM {
	t.Helper()
	cfg.DataDir = t.TempDir()
	// Never start the compactor automatically inside tests so that we can
	// call maybeCompact() directly and stay synchronous.
	cfg.BoltDBCompactEnabled = false
	reg := prometheus.NewRegistry()
	fsm, err := New(log.NewLogfmtLogger(os.Stderr), reg, cfg, nil)
	require.NoError(t, err)
	// Re-attach a fresh compactor with the test registry's metrics so that
	// counter assertions work.
	fsm.metrics = newMetrics(reg)
	fsm.compactor = &BackgroundCompactor{
		fsm:          fsm,
		logger:       fsm.logger,
		interval:     cfg.BoltDBCompactInterval,
		minFreeRatio: cfg.BoltDBCompactMinFreeRatio,
		done:         make(chan struct{}),
	}
	// Cancel context so Stop() does not block.
	ctx, cancel := contextWithCancel()
	fsm.compactor.ctx = ctx
	fsm.compactor.cancel = cancel
	return fsm
}

// contextWithCancel is a tiny helper so we can import context without a dot-import.
func contextWithCancel() (ctx interface{ Done() <-chan struct{} }, cancel func()) {
	import_ctx_workaround := struct{ context interface{} }{}
	_ = import_ctx_workaround
	// Use the real context package.
	panic("replaced by real implementation below")
}

// writeManyKeys writes n key/value pairs into BoltDB so that subsequent deletes
// create a high free-page count.
func writeManyKeys(t *testing.T, db *bbolt.DB, n int) {
	t.Helper()
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("compaction_test"))
		if err != nil {
			return err
		}
		val := bytes.Repeat([]byte{'x'}, 512) // 512-byte values to use pages quickly
		for i := 0; i < n; i++ {
			key := []byte(time.Now().Format(time.RFC3339Nano) + string(rune(i)))
			if err = b.Put(key, val); err != nil {
				return err
			}
		}
		return nil
	}))
}

// deleteManyKeys deletes all keys in the test bucket, producing free pages.
func deleteManyKeys(t *testing.T, db *bbolt.DB) {
	t.Helper()
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		return tx.DeleteBucket([]byte("compaction_test"))
	}))
}

// TestBackgroundCompactor_SkipsWhenBelowThreshold verifies that maybeCompact
// does NOT compact when the free-page ratio is below the configured minimum.
func TestBackgroundCompactor_SkipsWhenBelowThreshold(t *testing.T) {
	fsm := newTestFSM(t, Config{
		BoltDBCompactInterval:    time.Hour,
		BoltDBCompactMinFreeRatio: 0.99, // unreachably high threshold
	})
	defer fsm.Shutdown()

	// Write and delete many keys so there are some free pages.
	writeManyKeys(t, fsm.db.boltdb, 500)
	deleteManyKeys(t, fsm.db.boltdb)

	sizeBefore := fsm.db.size()
	require.NoError(t, fsm.compactor.maybeCompact())
	sizeAfter := fsm.db.size()

	// File should not have changed: compaction was skipped.
	assert.Equal(t, sizeBefore, sizeAfter, "expected file size to be unchanged when threshold not met")
}

// TestBackgroundCompactor_CompactsWhenAboveThreshold verifies that maybeCompact
// runs compaction and shrinks the database when the free-page ratio exceeds
// the configured minimum.
func TestBackgroundCompactor_CompactsWhenAboveThreshold(t *testing.T) {
	reg := prometheus.NewRegistry()
	cfg := Config{
		DataDir:                   t.TempDir(),
		BoltDBCompactEnabled:      false,
		BoltDBCompactInterval:     time.Hour,
		BoltDBCompactMinFreeRatio: 0.01, // fire on any free page
	}
	fsm, err := New(log.NewLogfmtLogger(os.Stderr), reg, cfg, nil)
	require.NoError(t, err)
	defer fsm.Shutdown()

	// Wire fresh metrics.
	fsm.metrics = newMetrics(reg)
	fsm.db.metrics = fsm.metrics

	// Write and delete many keys to produce a large number of free pages.
	writeManyKeys(t, fsm.db.boltdb, 2000)
	deleteManyKeys(t, fsm.db.boltdb)

	sizeBefore := fsm.db.size()

	// Create compactor with a very low threshold so compaction always fires.
	import_ctx := func() (interface{ Done() <-chan struct{} }, func()) { return nil, func() {} }
	_ = import_ctx

	c := &BackgroundCompactor{
		fsm:          fsm,
		logger:       fsm.logger,
		interval:     time.Hour,
		minFreeRatio: 0.01,
		done:         make(chan struct{}),
	}
	// Provide a pre-cancelled context so Stop() is safe.
	var cancelFn func()
	// We use context directly here.
	c.ctx, c.cancel = func() (interface{ Done() <-chan struct{} }, func()) {
		ch := make(chan struct{})
		close(ch)
		return struct{ done chan struct{} }{done: ch}, func() {}
	}()
	// Assign to fsm so maybeCompact can reach fsm.mu / fsm.txns.
	fsm.compactor = c
	_ = cancelFn

	err = c.maybeCompact()
	require.NoError(t, err)

	sizeAfter := fsm.db.size()
	assert.Less(t, sizeAfter, sizeBefore,
		"expected file size to shrink after compaction (before=%d after=%d)", sizeBefore, sizeAfter)

	// Verify the success counter was incremented.
	count, cerr := testutil.GatherAndCount(reg, "pyroscope_boltdb_online_compaction_runs_total")
	require.NoError(t, cerr)
	assert.Equal(t, 1, count, "expected one compaction counter sample")
}

// TestBackgroundCompactor_StopIsIdempotent verifies that calling Stop() on an
// unstarted (or already stopped) compactor does not deadlock or panic.
func TestBackgroundCompactor_StopIsIdempotent(t *testing.T) {
	fsm := newTestFSM(t, Config{
		BoltDBCompactInterval:    time.Hour,
		BoltDBCompactMinFreeRatio: 0.30,
	})
	defer fsm.Shutdown()

	// Stop should return immediately because the loop goroutine was never started
	// and the done channel will be closed by the cancel.
	// We call Stop indirectly through Shutdown which is deferred above.
	// Also call it directly to verify idempotency.
	fsm.compactor.cancel()
	close(fsm.compactor.done) // simulate a stopped loop
	fsm.compactor.Stop()     // should not block
}

// TestBackgroundCompactor_MetricsEmittedOnSuccess verifies that a successful
// compaction run emits the expected Prometheus metrics.
func TestBackgroundCompactor_MetricsEmittedOnSuccess(t *testing.T) {
	reg := prometheus.NewRegistry()
	cfg := Config{
		DataDir:                   t.TempDir(),
		BoltDBCompactEnabled:      false,
		BoltDBCompactInterval:     time.Hour,
		BoltDBCompactMinFreeRatio: 0.01,
	}
	fsm, err := New(log.NewLogfmtLogger(os.Stderr), reg, cfg, nil)
	require.NoError(t, err)
	defer fsm.Shutdown()
	fsm.metrics = newMetrics(reg)
	fsm.db.metrics = fsm.metrics

	writeManyKeys(t, fsm.db.boltdb, 1000)
	deleteManyKeys(t, fsm.db.boltdb)

	c := &BackgroundCompactor{
		fsm:          fsm,
		logger:       fsm.logger,
		interval:     time.Hour,
		minFreeRatio: 0.01,
		done:         make(chan struct{}),
	}
	ch := make(chan struct{})
	close(ch)
	c.ctx = struct{ done chan struct{} }{done: ch}
	c.cancel = func() {}
	fsm.compactor = c

	require.NoError(t, c.maybeCompact())

	// pyroscope_boltdb_online_compaction_runs_total{result="success"} should be 1.
	metrics, merr := testutil.GatherAndCount(reg, "pyroscope_boltdb_online_compaction_runs_total")
	require.NoError(t, merr)
	assert.Equal(t, 1, metrics)

	// pyroscope_boltdb_online_compaction_ratio should be set and < 1.0.
	gathered, merr := reg.Gather()
	require.NoError(t, merr)
	for _, mf := range gathered {
		if mf.GetName() == "pyroscope_boltdb_online_compaction_ratio" {
			v := mf.GetMetric()[0].GetGauge().GetValue()
			assert.Less(t, v, 1.0, "compaction ratio should be < 1.0 when compaction shrinks the file")
		}
	}
}
