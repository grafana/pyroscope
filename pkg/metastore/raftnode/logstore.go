package raftnode

import (
	"fmt"
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
// A timed-out write is abandoned, not cancelled, and may still land after
// raft has observed an error. writeToken ensures there is at most one such
// write: callers that time out waiting for the token never dispatch their
// write. Retries reconcile entries already persisted by an abandoned write
// before appending their new tail.
type timeoutLogStore struct {
	store        raft.LogStore
	timeout      time.Duration
	writeLatency prometheus.Histogram
	timeouts     prometheus.Counter

	writeToken       chan struct{}
	acknowledgedLast uint64
}

func newTimeoutLogStore(store raft.LogStore, timeout time.Duration, writeLatency prometheus.Histogram, timeouts prometheus.Counter) (raft.LogStore, error) {
	if timeout <= 0 {
		return store, nil
	}
	last, err := store.LastIndex()
	if err != nil {
		return nil, fmt.Errorf("read initial log store index: %w", err)
	}
	s := &timeoutLogStore{
		store:            store,
		timeout:          timeout,
		writeLatency:     writeLatency,
		timeouts:         timeouts,
		writeToken:       make(chan struct{}, 1),
		acknowledgedLast: last,
	}
	s.writeToken <- struct{}{}
	return s, nil
}

func (s *timeoutLogStore) FirstIndex() (uint64, error) { return s.store.FirstIndex() }
func (s *timeoutLogStore) LastIndex() (uint64, error)  { return s.store.LastIndex() }
func (s *timeoutLogStore) GetLog(index uint64, log *raft.Log) error {
	return s.store.GetLog(index, log)
}

// DeleteRange is not timed out: unlike an append, a delayed truncation cannot
// safely be allowed to land after raft has moved on.
func (s *timeoutLogStore) DeleteRange(min, max uint64) error {
	start := time.Now()
	timer := time.NewTimer(s.timeout)
	defer timer.Stop()
	if !s.acquireWriteToken(timer) {
		return fmt.Errorf("timed out waiting for in-flight log store write after %s", s.timeout)
	}
	if time.Since(start) >= s.timeout {
		s.writeToken <- struct{}{}
		return fmt.Errorf("timed out waiting for in-flight log store write after %s", s.timeout)
	}
	defer func() { s.writeToken <- struct{}{} }()

	lastIdx, err := s.store.LastIndex()
	if err != nil {
		return fmt.Errorf("failed to read last index: %w", err)
	}
	if max >= s.acknowledgedLast {
		// Raft calculates suffix deletions from its last acknowledged index.
		// Include any physical tail left by a timed-out write.
		if lastIdx > max {
			max = lastIdx
		}
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
	return s.withTimeout(func() error {
		return s.reconcileAndStore(logs)
	}, func() {
		if len(logs) > 0 {
			s.acknowledgedLast = logs[len(logs)-1].Index
		}
	})
}

// reconcileAndStore appends logs to the underlying store, first reconciling
// any overlap with entries already persisted by a previously abandoned
// write (see reconcileOverlap). It must be called while holding writeToken.
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
// into logs from which appending should resume. It must be called while
// holding writeToken, since it reads and truncates the store directly.
//
//   - entries with an index beyond lastIdx are new; the loop stops there
//     and that index is returned as-is.
//   - entries whose on-disk counterpart has the same term are dropped
//     (idempotent retry); the loop continues to the next entry.
//   - entries with a different term conflict; the on-disk suffix is truncated
//     and the incoming entry and its tail are appended in its place.
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
		if existing.Term == logs[i].Term {
			continue
		}
		if err := s.store.DeleteRange(logs[i].Index, lastIdx); err != nil {
			return 0, fmt.Errorf("failed to truncate conflicting tail from index %d: %w", logs[i].Index, err)
		}
		return i, nil
	}
	return i, nil
}

func (s *timeoutLogStore) withTimeout(fn func() error, onSuccess func()) error {
	start := time.Now()
	timer := time.NewTimer(s.timeout)
	defer timer.Stop()

	if !s.acquireWriteToken(timer) {
		s.observeTimeout(start)
		return fmt.Errorf("log store write timed out after %s", s.timeout)
	}
	if time.Since(start) >= s.timeout {
		s.writeToken <- struct{}{}
		s.observeTimeout(start)
		return fmt.Errorf("log store write timed out after %s", s.timeout)
	}

	done := make(chan error)
	abandoned := make(chan struct{})
	go func() {
		err := fn()
		select {
		case done <- err:
		case <-abandoned:
			s.writeToken <- struct{}{}
		}
	}()
	select {
	case err := <-done:
		if err == nil {
			onSuccess()
		}
		s.writeToken <- struct{}{}
		s.writeLatency.Observe(time.Since(start).Seconds())
		return err
	case <-timer.C:
		// Check if the operation completed concurrently with the timeout.
		// Go's select picks randomly when multiple cases are ready.
		select {
		case err := <-done:
			if err == nil {
				onSuccess()
			}
			s.writeToken <- struct{}{}
			s.writeLatency.Observe(time.Since(start).Seconds())
			return err
		default:
		}
		close(abandoned)
		s.observeTimeout(start)
		return fmt.Errorf("log store write timed out after %s", s.timeout)
	}
}

// acquireWriteToken returns false if the deadline expires before the caller
// owns the token. The second check handles the race where the token and timer
// become ready at the same time: no operation is dispatched after its budget
// has already elapsed.
func (s *timeoutLogStore) acquireWriteToken(timer *time.Timer) bool {
	select {
	case <-s.writeToken:
	case <-timer.C:
		return false
	}

	select {
	case <-timer.C:
		s.writeToken <- struct{}{}
		return false
	default:
		return true
	}
}

func (s *timeoutLogStore) observeTimeout(start time.Time) {
	s.writeLatency.Observe(time.Since(start).Seconds())
	s.timeouts.Inc()
}
