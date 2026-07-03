package raftnode

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus"
)

// timeoutLogStore wraps a raft.LogStore with a deadline on write operations.
// If the underlying store takes longer than the configured timeout, the
// operation returns an error instead of blocking indefinitely.
//
// This prevents a stuck disk (high I/O wait) from permanently stalling
// the raft leader. Without this, a blocked StoreLogs call freezes the
// leader's main goroutine while heartbeats continue on separate goroutines,
// preventing followers from ever triggering an election.
//
// A timed-out write is *abandoned*, not cancelled: the goroutine performing
// it keeps running and will eventually land its entries on disk regardless
// of what the caller (raft) does next. This desyncs raft's view of the log
// (which believes the append failed) from what's actually persisted, and
// creates two problems that timeoutLogStore must handle itself, because the
// underlying store (raft-wal) will not:
//
//   - The retry raft sends for the same index arrives after the abandoned
//     write already landed it. Monotonic stores like raft-wal reject the
//     re-append outright ("non-monotonic log entries"), which without
//     reconciliation here becomes a permanent livelock: every retry fails
//     the same way until the leader independently snapshots and forces an
//     InstallSnapshot.
//   - The retry can race the still-running abandoned write inside the
//     underlying store. raft-wal (like most LogStore implementations)
//     requires a single writer; concurrent Append calls are undefined
//     behavior.
//
// writeMu serializes all writes to the underlying store and is held for the
// reconciliation logic below: before appending, an overlapping prefix of the
// batch is compared against what's already on disk (by index and term) and
// either dropped (idempotent retry) or truncated (term conflict, e.g. from a
// stepped-down leader) so only the genuinely new tail is appended.
//
// DeleteRange must be serialized under writeMu too, for two reasons:
//
//   - In production this store is wrapped in a raft.LogCache (see node.go),
//     whose StoreLogs populates its ring-buffer cache only *after* the
//     underlying write succeeds, while its DeleteRange clears the cache
//     first and then deletes from the store, with no lock spanning both
//     steps. An abandoned write that is still inside LogCache.StoreLogs
//     when a DeleteRange runs on a different goroutine (e.g. compactLogs,
//     which raft runs on its own snapshot goroutine, concurrently with the
//     main loop) can populate the cache with entries *after* the delete
//     runs, serving reads that no longer match what's on disk. Holding
//     writeMu around the whole StoreLogs/DeleteRange call — including
//     LogCache's cache bookkeeping — closes that window.
//   - It orders DeleteRange against reconcileOverlap's own reads, so a
//     truncation can't happen mid-reconciliation.
//
// Serializing alone is not sufficient for a suffix/full truncation (raft's
// conflict-clearing DeleteRange, or removeOldLogs after InstallSnapshot):
// if an abandoned write's goroutine has not yet reached writeMu.Lock() by
// the time such a DeleteRange runs and completes (a real possibility, since
// nothing guarantees a just-spawned goroutine is scheduled promptly), the
// write proceeds afterwards against an empty or shortened log. Its indexes
// then look like new, past-the-end entries rather than an overlap, and
// reconcileOverlap has nothing on disk to compare them against — the stale
// batch gets appended as if it were current. epoch guards against this: a
// suffix/full DeleteRange bumps it, and every write captures the epoch
// *before* being dispatched to its goroutine, so a write whose captured
// epoch is stale by the time it reaches writeMu is rejected outright,
// however late it runs. Prefix compaction does not bump epoch: it targets
// indexes strictly below the last one and can run genuinely concurrently
// with legitimate in-flight writes to newer indexes, which must not be
// rejected.
type timeoutLogStore struct {
	store        raft.LogStore
	timeout      time.Duration
	writeLatency prometheus.Histogram
	timeouts     prometheus.Counter

	// writeMu is acquired *inside* the goroutine that performs the write,
	// not before spawning it. This way the timeout in withTimeout can still
	// fire and return promptly to raft even while a previous abandoned
	// write is still holding the lock; the next write simply waits for it
	// to finish (and can itself time out while waiting).
	writeMu sync.Mutex

	// epoch is bumped by every suffix/full DeleteRange (see comment above).
	// Writes capture it before dispatch and are rejected if it has moved by
	// the time they reach writeMu.
	epoch atomic.Uint64
}

func newTimeoutLogStore(store raft.LogStore, timeout time.Duration, writeLatency prometheus.Histogram, timeouts prometheus.Counter) raft.LogStore {
	if timeout <= 0 {
		return store
	}
	return &timeoutLogStore{
		store:        store,
		timeout:      timeout,
		writeLatency: writeLatency,
		timeouts:     timeouts,
	}
}

func (s *timeoutLogStore) FirstIndex() (uint64, error) { return s.store.FirstIndex() }
func (s *timeoutLogStore) LastIndex() (uint64, error)  { return s.store.LastIndex() }
func (s *timeoutLogStore) GetLog(index uint64, log *raft.Log) error {
	return s.store.GetLog(index, log)
}

// DeleteRange is serialized with writes under writeMu and bumps epoch for
// suffix/full truncations (max reaching the current last index) to reject
// stale abandoned writes that were dispatched before the truncation. See
// the timeoutLogStore doc comment for the full rationale. It is not itself
// subject to the write timeout: an abandoned truncation landing late would
// be worse than raft's main loop blocking on a slow one.
func (s *timeoutLogStore) DeleteRange(min, max uint64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	lastIdx, err := s.store.LastIndex()
	if err != nil {
		return fmt.Errorf("failed to read last index: %w", err)
	}
	if max >= lastIdx {
		// Suffix or full truncation: raft's conflict-clearing DeleteRange
		// (raft.go) and removeOldLogs after InstallSnapshot both clear up
		// to (at least) the current last index. Prefix compaction
		// (compactLogs) always targets a strictly older range and must not
		// invalidate writes to newer indexes that may legitimately be in
		// flight concurrently.
		s.epoch.Add(1)
	}
	return s.store.DeleteRange(min, max)
}

// IsMonotonic implements raft.MonotonicLogStore by delegating to the
// underlying store. Without this, raft uses compactLogs (which retains
// TrailingLogs) instead of removeOldLogs after snapshot install on a
// follower — leaving stale WAL entries that cause non-monotonic index
// errors when the leader resumes replication.
func (s *timeoutLogStore) IsMonotonic() bool {
	if m, ok := s.store.(raft.MonotonicLogStore); ok {
		return m.IsMonotonic()
	}
	return false
}

func (s *timeoutLogStore) StoreLog(log *raft.Log) error {
	return s.StoreLogs([]*raft.Log{log})
}

func (s *timeoutLogStore) StoreLogs(logs []*raft.Log) error {
	// Captured before dispatch, on the caller's goroutine, so it reflects
	// the epoch at the moment raft issued the write rather than whenever
	// the write's goroutine eventually gets scheduled. See the
	// timeoutLogStore doc comment.
	epoch := s.epoch.Load()
	return s.withTimeout(func() error {
		return s.storeLogsAtEpoch(logs, epoch)
	})
}

// storeLogsAtEpoch performs the actual write, rejecting it outright if
// epoch has moved past the value captured by StoreLogs before dispatch
// (see the timeoutLogStore doc comment). It is split out from StoreLogs so
// the abandoned-write-after-truncation scenario can be driven directly and
// deterministically in tests, since it does not otherwise depend on
// goroutine scheduling.
func (s *timeoutLogStore) storeLogsAtEpoch(logs []*raft.Log, epoch uint64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if current := s.epoch.Load(); current != epoch {
		return fmt.Errorf("log store write abandoned: log was truncated (epoch %d != %d) since this write was issued", current, epoch)
	}
	return s.reconcileAndStore(logs)
}

// reconcileAndStore appends logs to the underlying store, first reconciling
// any overlap with entries already persisted by a previously abandoned
// write (see reconcileOverlap). It must be called with writeMu held.
func (s *timeoutLogStore) reconcileAndStore(logs []*raft.Log) error {
	if len(logs) == 0 {
		return nil
	}

	lastIdx, err := s.store.LastIndex()
	if err != nil {
		return fmt.Errorf("failed to read last index: %w", err)
	}

	i, err := s.reconcileOverlap(logs, lastIdx)
	if err != nil {
		return err
	}

	remaining := logs[i:]
	if len(remaining) == 0 {
		// The whole batch was an idempotent retry of what's already on
		// disk; nothing left to do.
		return nil
	}
	return s.store.StoreLogs(remaining)
}

// reconcileOverlap compares the leading entries of logs against what's
// already persisted at their indexes (up to lastIdx) and returns the index
// into logs from which appending should resume. It must be called with
// writeMu held, since it reads and truncates the underlying store directly.
//
//   - entries with an index beyond lastIdx are new; the loop stops there
//     and that index is returned as-is.
//   - entries whose on-disk counterpart has the same term are dropped
//     (idempotent retry); the loop continues to the next entry.
//   - an entry whose on-disk counterpart has a lower term wins: the stale
//     tail on disk is truncated via DeleteRange and that entry's index is
//     returned, so it (and the rest of the batch) is appended as the new
//     tail.
//   - an entry whose on-disk counterpart has a higher term is itself stale
//     (e.g. a since-superseded abandoned write only now getting its turn)
//     and is rejected: per raft's log matching property, an entry can only
//     replace one at the same index by carrying a strictly higher term, so
//     it must not clobber the newer entry already on disk.
func (s *timeoutLogStore) reconcileOverlap(logs []*raft.Log, lastIdx uint64) (int, error) {
	i := 0
	for ; i < len(logs) && logs[i].Index <= lastIdx; i++ {
		var existing raft.Log
		if err := s.store.GetLog(logs[i].Index, &existing); err != nil {
			// This includes the case where the index has already been
			// compacted away below FirstIndex (e.g. a snapshot install
			// raced this reconciliation): there's nothing to compare
			// against, so surface the error and let raft retry rather than
			// guessing at intent.
			return 0, fmt.Errorf("failed to read existing log at index %d: %w", logs[i].Index, err)
		}
		switch {
		case existing.Term == logs[i].Term:
			continue
		case logs[i].Term < existing.Term:
			return 0, fmt.Errorf("log store index %d: on-disk term %d is newer than incoming term %d, refusing to overwrite",
				logs[i].Index, existing.Term, logs[i].Term)
		default:
			if err := s.store.DeleteRange(logs[i].Index, lastIdx); err != nil {
				return 0, fmt.Errorf("failed to truncate conflicting tail from index %d: %w", logs[i].Index, err)
			}
			return i, nil
		}
	}
	return i, nil
}

func (s *timeoutLogStore) withTimeout(fn func() error) error {
	start := time.Now()
	done := make(chan error, 1)
	go func() {
		done <- fn()
	}()
	select {
	case err := <-done:
		s.writeLatency.Observe(time.Since(start).Seconds())
		return err
	case <-time.After(s.timeout):
		// Check if the operation completed concurrently with the timeout.
		// Go's select picks randomly when multiple cases are ready.
		select {
		case err := <-done:
			s.writeLatency.Observe(time.Since(start).Seconds())
			return err
		default:
		}
		s.writeLatency.Observe(time.Since(start).Seconds())
		s.timeouts.Inc()
		return fmt.Errorf("log store write timed out after %s", s.timeout)
	}
}
