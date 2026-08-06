package raftnode

import (
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	raftwal "github.com/hashicorp/raft-wal"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

// gatedLogStore wraps a raft.LogStore and lets tests hold StoreLogs/StoreLog
// calls open on a channel, so a write can be started, "abandoned" by a
// timeout wrapper, and completed later out of band. This simulates a slow
// disk: the write is real and eventually lands, but the caller (raft, via
// timeoutLogStore) may have already given up waiting for it.
type gatedLogStore struct {
	store raft.LogStore

	mu   sync.Mutex
	gate chan struct{}

	// holdDuration, if set, is slept once a call has passed the gate but
	// before it reaches the real underlying store, simulating a slow disk
	// write that stays "in flight" for a known, deterministic window. This
	// lets tests reliably observe (or fail to observe) overlapping calls
	// without depending on goroutine-scheduling luck.
	holdDuration time.Duration

	waiting     int32 // number of callers currently parked at the gate
	writes      int32 // number of StoreLogs/StoreLog calls currently inside the underlying store
	maxInFlight int32

	intervals []interval // [start,end) of every completed StoreLogs call into the underlying store
}

type interval struct {
	start, end time.Time
}

// overlaps reports whether any two recorded intervals overlap in time,
// which would indicate two writes were concurrently inside the underlying
// store — a single-writer violation.
func (g *gatedLogStore) overlaps() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	for i := range g.intervals {
		for j := range g.intervals {
			if i == j {
				continue
			}
			a, b := g.intervals[i], g.intervals[j]
			if a.start.Before(b.end) && b.start.Before(a.end) {
				return true
			}
		}
	}
	return false
}

func newGatedLogStore(store raft.LogStore) *gatedLogStore {
	return &gatedLogStore{store: store}
}

// arm makes the next write block until release is called (or the returned
// channel is closed).
func (g *gatedLogStore) arm() chan struct{} {
	ch := make(chan struct{})
	g.mu.Lock()
	g.gate = ch
	g.mu.Unlock()
	return ch
}

// release unblocks a write previously armed with arm.
func (g *gatedLogStore) release(ch chan struct{}) {
	close(ch)
	g.mu.Lock()
	if g.gate == ch {
		g.gate = nil
	}
	g.mu.Unlock()
}

func (g *gatedLogStore) waitAtGate() {
	g.mu.Lock()
	ch := g.gate
	g.mu.Unlock()
	if ch != nil {
		g.mu.Lock()
		g.waiting++
		g.mu.Unlock()
		<-ch
		g.mu.Lock()
		g.waiting--
		g.mu.Unlock()
	}
}

func (g *gatedLogStore) waitersAtGate() int32 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.waiting
}

func (g *gatedLogStore) FirstIndex() (uint64, error) { return g.store.FirstIndex() }
func (g *gatedLogStore) LastIndex() (uint64, error)  { return g.store.LastIndex() }
func (g *gatedLogStore) GetLog(index uint64, log *raft.Log) error {
	return g.store.GetLog(index, log)
}
func (g *gatedLogStore) DeleteRange(min, max uint64) error { return g.store.DeleteRange(min, max) }

func (g *gatedLogStore) IsMonotonic() bool {
	if m, ok := g.store.(raft.MonotonicLogStore); ok {
		return m.IsMonotonic()
	}
	return false
}

func (g *gatedLogStore) StoreLog(log *raft.Log) error {
	return g.StoreLogs([]*raft.Log{log})
}

func (g *gatedLogStore) StoreLogs(logs []*raft.Log) error {
	g.waitAtGate()
	g.trackInFlight(1)
	defer g.trackInFlight(-1)
	start := time.Now()
	if g.holdDuration > 0 {
		// Simulate the on-disk write taking a known amount of time, so
		// concurrent calls that pass the gate together have a deterministic
		// window in which to overlap inside the underlying store.
		time.Sleep(g.holdDuration)
	}
	err := g.store.StoreLogs(logs)
	g.mu.Lock()
	g.intervals = append(g.intervals, interval{start: start, end: time.Now()})
	g.mu.Unlock()
	return err
}

func (g *gatedLogStore) trackInFlight(delta int32) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.writes += delta
	if g.writes > g.maxInFlight {
		g.maxInFlight = g.writes
	}
}

func (g *gatedLogStore) maxConcurrentWrites() int32 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.maxInFlight
}

func testMetrics() (prometheus.Histogram, prometheus.Counter) {
	return prometheus.NewHistogram(prometheus.HistogramOpts{Name: "test_write_latency"}),
		prometheus.NewCounter(prometheus.CounterOpts{Name: "test_timeouts"})
}

// entry builds a minimal raft.Log for use in these tests.
func entry(index, term uint64) *raft.Log {
	return &raft.Log{Index: index, Term: term, Type: raft.LogCommand, Data: []byte("x")}
}

// TestTimeoutLogStore_TimeoutThenRetryLivelock reproduces the follower
// livelock this fix addresses:
//
//  1. A StoreLogs call for index N is slow; timeoutLogStore gives up on it
//     and returns an error to raft, but the underlying raft-wal write is
//     *not* cancelled and eventually lands entry N on disk anyway.
//  2. Raft, believing the append failed, retries StoreLogs for the same
//     index N.
//  3. Against a real raft-wal store (which enforces strict monotonicity and
//     refuses to re-append an index it already has), the retry fails with
//     "non-monotonic log entries", forever — the node is stuck reporting
//     "too far behind" until the leader falls back to InstallSnapshot.
//
// A correct wrapper must reconcile the desync itself: the retry for an
// already-persisted, identical index should be treated as a success (or
// merged/truncated on conflict), so that StoreLogs(N) returns nil and
// replication can continue with N+1.
//
// This pins down the bug: it fails against a naive wrapper that just
// re-appends, and passes against timeoutLogStore's reconciliation logic.
func TestTimeoutLogStore_TimeoutThenRetryLivelock(t *testing.T) {
	dir := t.TempDir()
	wal, err := raftwal.Open(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = wal.Close() })

	gated := newGatedLogStore(wal)
	writeLatency, timeouts := testMetrics()
	ls := newTimeoutLogStore(gated, 50*time.Millisecond, writeLatency, timeouts)

	const N = uint64(1)

	// Step 1: the write for index N is slow. Arm the gate so the underlying
	// raft-wal write blocks; timeoutLogStore should give up after 50ms and
	// return an error to the caller (simulating raft observing the append
	// as failed), even though the real write is still in flight.
	gate := gated.arm()

	timeoutErr := make(chan error, 1)
	go func() {
		timeoutErr <- ls.StoreLogs([]*raft.Log{entry(N, 1)})
	}()

	select {
	case err := <-timeoutErr:
		require.Error(t, err, "expected the slow write to time out")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the abandoned write to report a timeout")
	}

	// Step 2: release the blocked write and let it land on disk. This is
	// the "phantom persist": raft believes the append failed, but the WAL
	// now actually has index N.
	gated.release(gate)
	require.Eventually(t, func() bool {
		last, err := wal.LastIndex()
		return err == nil && last == N
	}, 5*time.Second, 10*time.Millisecond, "the abandoned write never landed on disk")

	// Step 3: raft retries AppendEntries for the same index N. A correct
	// implementation reconciles this with what's already on disk instead
	// of blindly re-appending and hitting raft-wal's monotonicity check.
	retryErr := ls.StoreLogs([]*raft.Log{entry(N, 1)})
	require.NoError(t, retryErr, "retry of an already-persisted identical entry must not fail")

	last, err := wal.LastIndex()
	require.NoError(t, err)
	require.Equal(t, N, last)

	// Step 4: replication must be able to continue with the next index.
	require.NoError(t, ls.StoreLogs([]*raft.Log{entry(N+1, 1)}))
	last, err = wal.LastIndex()
	require.NoError(t, err)
	require.Equal(t, N+1, last)
}

// TestTimeoutLogStore_TermConflictTruncates reproduces the stepped-down
// leader variant: the retry for an overlapping index carries a *different*
// (higher) term than what is already on disk. A correct wrapper must
// truncate the conflicting tail and append the new entry, rather than
// treating it as an idempotent duplicate or failing outright.
func TestTimeoutLogStore_TermConflictTruncates(t *testing.T) {
	dir := t.TempDir()
	wal, err := raftwal.Open(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = wal.Close() })

	gated := newGatedLogStore(wal)
	writeLatency, timeouts := testMetrics()
	// The retry path here does more work than a plain append (it reads the
	// existing entry and truncates before re-appending), so give it a more
	// generous timeout than the bare-minimum livelock test; the point of
	// this test is the reconciliation logic, not the timeout value itself.
	ls := newTimeoutLogStore(gated, 500*time.Millisecond, writeLatency, timeouts)

	const N = uint64(1)

	gate := gated.arm()
	timeoutErr := make(chan error, 1)
	go func() {
		timeoutErr <- ls.StoreLogs([]*raft.Log{entry(N, 1)})
	}()
	select {
	case err := <-timeoutErr:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the abandoned write to report a timeout")
	}

	gated.release(gate)
	require.Eventually(t, func() bool {
		last, err := wal.LastIndex()
		return err == nil && last == N
	}, 5*time.Second, 10*time.Millisecond)

	// A new leader (higher term) now retries with a different entry at the
	// same index N.
	require.NoError(t, ls.StoreLogs([]*raft.Log{entry(N, 2)}))

	var got raft.Log
	require.NoError(t, wal.GetLog(N, &got))
	require.Equal(t, uint64(2), got.Term, "entry at N must reflect the new leader's term")
}

// TestTimeoutLogStore_PartialOverlapAppendsOnlyNewTail exercises a batch
// where only a *prefix* of the entries is already persisted (from a
// previously abandoned write) and the rest is genuinely new. Only the
// already-persisted prefix must be dropped; the new tail must still be
// appended in the same call.
func TestTimeoutLogStore_PartialOverlapAppendsOnlyNewTail(t *testing.T) {
	dir := t.TempDir()
	wal, err := raftwal.Open(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = wal.Close() })

	gated := newGatedLogStore(wal)
	writeLatency, timeouts := testMetrics()
	ls := newTimeoutLogStore(gated, 50*time.Millisecond, writeLatency, timeouts)

	// Abandoned write lands indexes 1 and 2 on disk, but the caller (raft)
	// observed a timeout and believes the append failed.
	gate := gated.arm()
	timeoutErr := make(chan error, 1)
	go func() {
		timeoutErr <- ls.StoreLogs([]*raft.Log{entry(1, 1), entry(2, 1)})
	}()
	select {
	case err := <-timeoutErr:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the abandoned write to report a timeout")
	}
	gated.release(gate)
	require.Eventually(t, func() bool {
		last, err := wal.LastIndex()
		return err == nil && last == 2
	}, 5*time.Second, 10*time.Millisecond)

	// Raft retries with the full batch it originally intended to send:
	// indexes 1-2 (already on disk, same term) followed by the genuinely
	// new indexes 3-4.
	require.NoError(t, ls.StoreLogs([]*raft.Log{
		entry(1, 1), entry(2, 1), entry(3, 1), entry(4, 1),
	}))

	last, err := wal.LastIndex()
	require.NoError(t, err)
	require.Equal(t, uint64(4), last, "the new tail must be appended alongside the already-persisted prefix")

	for idx := uint64(1); idx <= 4; idx++ {
		var got raft.Log
		require.NoError(t, wal.GetLog(idx, &got))
		require.Equal(t, uint64(1), got.Term)
	}
}

// TestTimeoutLogStore_StaleTermRejected verifies that reconcileAndStore
// refuses to overwrite an on-disk entry with an incoming entry that carries
// a *lower* term at the same index. This can happen when an abandoned
// write (from a leader that has since been superseded) is still queued
// behind writeMu and only gets its turn after a newer leader's entry has
// already been persisted at that index. Per raft's log matching property,
// only a strictly higher term may replace an entry; a lower term must never
// clobber what's already there.
func TestTimeoutLogStore_StaleTermRejected(t *testing.T) {
	dir := t.TempDir()
	wal, err := raftwal.Open(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = wal.Close() })

	gated := newGatedLogStore(wal)
	writeLatency, timeouts := testMetrics()
	ls := newTimeoutLogStore(gated, 50*time.Millisecond, writeLatency, timeouts)

	const N = uint64(1)

	// A newer leader's entry is already persisted at index N with term 2.
	require.NoError(t, ls.StoreLogs([]*raft.Log{entry(N, 2)}))

	// A stale write for the same index, carrying an older term, arrives
	// afterwards (e.g. a delayed abandoned write from the deposed leader).
	err = ls.StoreLogs([]*raft.Log{entry(N, 1)})
	require.Error(t, err, "a stale, lower-term entry must not be accepted")

	var got raft.Log
	require.NoError(t, wal.GetLog(N, &got))
	require.Equal(t, uint64(2), got.Term, "the newer on-disk entry must not be clobbered by the stale retry")
}

// TestTimeoutLogStore_SerializesAbandonedWrite verifies that once a write
// has been abandoned by the timeout, a subsequent StoreLogs call does not
// race with it inside the underlying store. raft-wal requires a single
// writer; two concurrent Append calls are undefined behavior and a path to
// WAL corruption.
//
// The abandoned write is held at the gate (still "in flight" as far as the
// underlying store is concerned) while a second write is issued. The second
// write must not enter the underlying store until the first one, once
// released, has fully completed — i.e. a correct wrapper serializes on the
// underlying store rather than letting the retry run concurrently with the
// stray goroutine from the timed-out call.
func TestTimeoutLogStore_SerializesAbandonedWrite(t *testing.T) {
	dir := t.TempDir()
	wal, err := raftwal.Open(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = wal.Close() })

	gated := newGatedLogStore(wal)
	// Once released, hold the underlying store for a known duration so a
	// second write that (incorrectly) entered concurrently has a wide
	// window in which to be observed overlapping.
	gated.holdDuration = 200 * time.Millisecond
	writeLatency, timeouts := testMetrics()
	ls := newTimeoutLogStore(gated, 50*time.Millisecond, writeLatency, timeouts)

	gate := gated.arm()

	// The abandoned write: timeoutLogStore gives up after 50ms, but its
	// spawned goroutine is still blocked at the gate, about to enter the
	// (simulated) underlying store.
	timeoutErr := make(chan error, 1)
	go func() {
		timeoutErr <- ls.StoreLogs([]*raft.Log{entry(1, 1)})
	}()
	select {
	case err := <-timeoutErr:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the abandoned write to report a timeout")
	}

	// The retry, issued while the first write is still blocked. A correct
	// wrapper serializes internally, so this call must not even reach the
	// underlying store's gate until the first write completes; an unfixed
	// wrapper lets it straight through, so it also parks at the (still
	// armed) gate alongside the first call.
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- ls.StoreLogs([]*raft.Log{entry(2, 1)})
	}()

	// Give the retry a chance to (incorrectly) reach the gate before we
	// release the first write.
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, int32(1), gated.waitersAtGate(),
		"the retry must not reach the underlying store's gate while the abandoned write is still blocked there")

	gated.release(gate)

	select {
	case <-secondDone:
	case <-time.After(5 * time.Second):
		t.Fatal("second write never completed")
	}

	require.LessOrEqual(t, gated.maxConcurrentWrites(), int32(1),
		"at most one write must be in flight in the underlying store at a time")
	require.False(t, gated.overlaps(),
		"no two writes may have been concurrently inside the underlying store")
}

// TestTimeoutLogStore_DeleteRangeWaitsForInFlightWrite verifies that
// DeleteRange is serialized with StoreLogs under writeMu: it must not enter
// the underlying store while an abandoned write is still holding the lock.
//
// This matters beyond the underlying store's own single-writer contract:
// in production this store is wrapped in a raft.LogCache (see node.go),
// whose StoreLogs populates its cache only after the underlying write
// succeeds and whose DeleteRange clears the cache before deleting from the
// store, with no lock spanning both steps. If DeleteRange could run while
// an abandoned write is still inside the (LogCache-wrapped) store, the
// write could populate the cache with entries after the delete has already
// cleared it, serving reads that no longer match what's on disk.
func TestTimeoutLogStore_DeleteRangeWaitsForInFlightWrite(t *testing.T) {
	dir := t.TempDir()
	wal, err := raftwal.Open(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = wal.Close() })

	gated := newGatedLogStore(wal)
	writeLatency, timeouts := testMetrics()
	// A generous timeout: the first write to a fresh WAL creates segment
	// files and can occasionally exceed a bare-minimum timeout under load
	// (e.g. -race), independent of the reconciliation logic under test.
	ls := newTimeoutLogStore(gated, 500*time.Millisecond, writeLatency, timeouts)

	require.NoError(t, ls.StoreLogs([]*raft.Log{entry(1, 1), entry(2, 1)}))

	// Abandon a write for index 3: timeoutLogStore gives up after the
	// timeout, but its goroutine is still blocked at the gate, holding
	// writeMu, about to enter the underlying store.
	gate := gated.arm()
	timeoutErr := make(chan error, 1)
	go func() {
		timeoutErr <- ls.StoreLogs([]*raft.Log{entry(3, 1)})
	}()
	select {
	case err := <-timeoutErr:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the abandoned write to report a timeout")
	}

	// DeleteRange must not proceed while the abandoned write still holds
	// writeMu.
	deleteDone := make(chan error, 1)
	go func() {
		deleteDone <- ls.DeleteRange(1, 2)
	}()

	select {
	case <-deleteDone:
		t.Fatal("DeleteRange must not complete while an abandoned write still holds writeMu")
	case <-time.After(200 * time.Millisecond):
	}

	gated.release(gate)

	select {
	case err := <-deleteDone:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("DeleteRange never completed after the abandoned write was released")
	}
}

// TestTimeoutLogStore_EpochRejectsWriteDispatchedBeforeTruncation covers
// the residual hazard that serializing on writeMu alone does not close: an
// abandoned write's goroutine that has not yet reached writeMu.Lock() by
// the time a suffix/full DeleteRange (raft's conflict-clearing truncation,
// or removeOldLogs after InstallSnapshot) runs and completes. Nothing
// guarantees a just-spawned goroutine is scheduled promptly, so without a
// guard the write can proceed afterwards against an empty or shortened
// log: its indexes look like new, past-the-end entries rather than an
// overlap, and reconcileOverlap has nothing on disk to compare them
// against — the stale batch would be appended as if current, resurrecting
// data a newer leader already discarded.
//
// This drives storeLogsAtEpoch directly with an epoch captured before the
// truncation, simulating exactly that scheduling delay without depending
// on goroutine-scheduling luck.
func TestTimeoutLogStore_EpochRejectsWriteDispatchedBeforeTruncation(t *testing.T) {
	dir := t.TempDir()
	wal, err := raftwal.Open(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = wal.Close() })

	writeLatency, timeouts := testMetrics()
	// See TestTimeoutLogStore_DeleteRangeWaitsForInFlightWrite for why this
	// is more generous than the bare-minimum timeout used elsewhere.
	ls := newTimeoutLogStore(wal, 500*time.Millisecond, writeLatency, timeouts)
	impl := ls.(*timeoutLogStore)

	require.NoError(t, ls.StoreLogs([]*raft.Log{entry(1, 1)}))
	staleEpoch := impl.epoch.Load()

	// A new leader's InstallSnapshot wipes the log — a suffix/full
	// truncation that must invalidate any write dispatched before it.
	require.NoError(t, ls.DeleteRange(1, 1))
	require.NotEqual(t, staleEpoch, impl.epoch.Load(), "a suffix/full DeleteRange must bump epoch")

	// The abandoned write for the old index, dispatched (and its epoch
	// captured) before the truncation, only now reaches writeMu.
	err = impl.storeLogsAtEpoch([]*raft.Log{entry(1, 1)}, staleEpoch)
	require.Error(t, err, "a write captured at a stale epoch must be rejected")

	last, err := wal.LastIndex()
	require.NoError(t, err)
	require.Equal(t, uint64(0), last, "the stale write must not have been appended into the truncated log")

	// Replication can continue normally afterwards.
	require.NoError(t, ls.StoreLogs([]*raft.Log{entry(1, 2)}))
}

// TestTimeoutLogStore_PrefixDeleteRangeDoesNotBumpEpoch verifies that a
// prefix compaction (raft's compactLogs, which runs on its own snapshot
// goroutine concurrently with the main loop) does not invalidate
// legitimately in-flight writes to newer indexes. Only suffix/full
// truncations, which discard the tail a write might target, need to bump
// epoch.
func TestTimeoutLogStore_PrefixDeleteRangeDoesNotBumpEpoch(t *testing.T) {
	dir := t.TempDir()
	wal, err := raftwal.Open(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = wal.Close() })

	writeLatency, timeouts := testMetrics()
	// See TestTimeoutLogStore_DeleteRangeWaitsForInFlightWrite for why this
	// is more generous than the bare-minimum timeout used elsewhere.
	ls := newTimeoutLogStore(wal, 500*time.Millisecond, writeLatency, timeouts)
	impl := ls.(*timeoutLogStore)

	require.NoError(t, ls.StoreLogs([]*raft.Log{entry(1, 1), entry(2, 1), entry(3, 1)}))
	epoch := impl.epoch.Load()

	// Prefix compaction: truncate index 1 only, well short of the last
	// index (3).
	require.NoError(t, ls.DeleteRange(1, 1))
	require.Equal(t, epoch, impl.epoch.Load(), "prefix compaction must not bump epoch")

	// A write dispatched before the compaction (captured the old epoch)
	// must still succeed.
	require.NoError(t, impl.storeLogsAtEpoch([]*raft.Log{entry(4, 1)}, epoch))

	last, err := wal.LastIndex()
	require.NoError(t, err)
	require.Equal(t, uint64(4), last)
}
