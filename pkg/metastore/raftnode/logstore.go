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
type timeoutLogStore struct {
	store        raft.LogStore
	timeout      time.Duration
	writeLatency prometheus.Histogram
	timeouts     prometheus.Counter
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
func (s *timeoutLogStore) DeleteRange(min, max uint64) error {
	return s.store.DeleteRange(min, max)
}

func (s *timeoutLogStore) StoreLog(log *raft.Log) error {
	return s.withTimeout(func() error {
		return s.store.StoreLog(log)
	})
}

func (s *timeoutLogStore) StoreLogs(logs []*raft.Log) error {
	return s.withTimeout(func() error {
		return s.store.StoreLogs(logs)
	})
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
