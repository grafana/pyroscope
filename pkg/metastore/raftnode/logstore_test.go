package raftnode

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	raftwal "github.com/hashicorp/raft-wal"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

type writeGate struct {
	release chan struct{}
	entered chan struct{}

	releaseOnce sync.Once
	enteredOnce sync.Once
}

func (g *writeGate) unblock() {
	g.releaseOnce.Do(func() { close(g.release) })
}

// gatedLogStore records calls and can block writes until its current gate is
// released. It models a disk write that continues after the caller times out.
type gatedLogStore struct {
	store raft.LogStore

	mu      sync.Mutex
	gate    *writeGate
	writes  [][]raft.Log
	deletes [][2]uint64
}

func newGatedLogStore(store raft.LogStore) *gatedLogStore {
	return &gatedLogStore{store: store}
}

func (g *gatedLogStore) arm() *writeGate {
	gate := &writeGate{release: make(chan struct{}), entered: make(chan struct{})}
	g.mu.Lock()
	g.gate = gate
	g.mu.Unlock()
	return gate
}

func (g *gatedLogStore) StoreLog(log *raft.Log) error {
	return g.StoreLogs([]*raft.Log{log})
}

func (g *gatedLogStore) StoreLogs(logs []*raft.Log) error {
	copyLogs := make([]raft.Log, len(logs))
	for i := range logs {
		copyLogs[i] = *logs[i]
	}

	g.mu.Lock()
	g.writes = append(g.writes, copyLogs)
	gate := g.gate
	g.mu.Unlock()
	if gate != nil {
		gate.enteredOnce.Do(func() { close(gate.entered) })
		<-gate.release
	}
	return g.store.StoreLogs(logs)
}

func (g *gatedLogStore) DeleteRange(min, max uint64) error {
	g.mu.Lock()
	g.deletes = append(g.deletes, [2]uint64{min, max})
	g.mu.Unlock()
	return g.store.DeleteRange(min, max)
}

func (g *gatedLogStore) FirstIndex() (uint64, error) { return g.store.FirstIndex() }
func (g *gatedLogStore) LastIndex() (uint64, error)  { return g.store.LastIndex() }
func (g *gatedLogStore) GetLog(index uint64, log *raft.Log) error {
	return g.store.GetLog(index, log)
}

func (g *gatedLogStore) IsMonotonic() bool {
	if m, ok := g.store.(raft.MonotonicLogStore); ok {
		return m.IsMonotonic()
	}
	return false
}

func (g *gatedLogStore) writeCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.writes)
}

func (g *gatedLogStore) deleteCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.deletes)
}

type lastIndexErrorStore struct {
	raft.LogStore
	err error
}

func (s lastIndexErrorStore) LastIndex() (uint64, error) { return 0, s.err }

func testMetrics() (prometheus.Histogram, prometheus.Counter) {
	return prometheus.NewHistogram(prometheus.HistogramOpts{Name: "test_write_latency"}),
		prometheus.NewCounter(prometheus.CounterOpts{Name: "test_timeouts"})
}

func newTestLogStore(t *testing.T, store raft.LogStore, timeout time.Duration) (*timeoutLogStore, prometheus.Counter) {
	t.Helper()
	writeLatency, timeouts := testMetrics()
	logStore, err := newTimeoutLogStore(store, timeout, writeLatency, timeouts)
	require.NoError(t, err)
	return logStore.(*timeoutLogStore), timeouts
}

func openWAL(t *testing.T) *raftwal.WAL {
	t.Helper()
	wal, err := raftwal.Open(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, wal.Close()) })
	return wal
}

func entry(index, term uint64) *raft.Log {
	return &raft.Log{Index: index, Term: term, Type: raft.LogCommand, Data: []byte("x")}
}

func entries(from, to, term uint64) []*raft.Log {
	logs := make([]*raft.Log, 0, to-from+1)
	for index := from; index <= to; index++ {
		logs = append(logs, entry(index, term))
	}
	return logs
}

func waitForGate(t *testing.T, gate *writeGate) {
	t.Helper()
	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for write to enter gate")
	}
}

func waitForLastIndex(t *testing.T, store raft.LogStore, want uint64) {
	t.Helper()
	require.Eventually(t, func() bool {
		last, err := store.LastIndex()
		return err == nil && last == want
	}, 5*time.Second, 10*time.Millisecond)
}

func TestNewTimeoutLogStore_LastIndexError(t *testing.T) {
	wantErr := errors.New("last index failed")
	writeLatency, timeouts := testMetrics()
	_, err := newTimeoutLogStore(lastIndexErrorStore{LogStore: newGatedLogStore(openWAL(t)), err: wantErr}, time.Second, writeLatency, timeouts)
	require.ErrorIs(t, err, wantErr)
}

func TestTimeoutLogStore_AbandonedWriteRetry(t *testing.T) {
	wal := openWAL(t)
	gated := newGatedLogStore(wal)
	logStore, timeouts := newTestLogStore(t, gated, 75*time.Millisecond)
	gate := gated.arm()
	t.Cleanup(gate.unblock)

	errCh := make(chan error, 1)
	go func() { errCh <- logStore.StoreLogs([]*raft.Log{entry(1, 1)}) }()
	waitForGate(t, gate)
	require.ErrorContains(t, <-errCh, "log store write timed out")
	require.Equal(t, float64(1), testutil.ToFloat64(timeouts))

	gate.unblock()
	waitForLastIndex(t, wal, 1)
	require.NoError(t, logStore.StoreLogs([]*raft.Log{entry(1, 1)}))
	require.NoError(t, logStore.StoreLogs([]*raft.Log{entry(2, 1)}))
	waitForLastIndex(t, wal, 2)
	require.Equal(t, float64(1), testutil.ToFloat64(timeouts))
}

func TestTimeoutLogStore_ReconcileOverlap(t *testing.T) {
	tests := []struct {
		name        string
		physical    []*raft.Log
		incoming    []*raft.Log
		wantTerms   map[uint64]uint64
		missingFrom uint64
	}{
		{
			name:      "partial identical overlap appends tail",
			physical:  entries(3, 4, 1),
			incoming:  append(entries(3, 4, 1), entries(5, 6, 1)...),
			wantTerms: map[uint64]uint64{1: 1, 2: 1, 3: 1, 4: 1, 5: 1, 6: 1},
		},
		{
			name:        "conflict replaces physical suffix",
			physical:    entries(3, 5, 1),
			incoming:    entries(3, 4, 2),
			wantTerms:   map[uint64]uint64{1: 1, 2: 1, 3: 2, 4: 2},
			missingFrom: 5,
		},
		{
			name:        "conflict after identical prefix replaces suffix",
			physical:    entries(3, 5, 1),
			incoming:    append(entries(3, 3, 1), entries(4, 5, 2)...),
			wantTerms:   map[uint64]uint64{1: 1, 2: 1, 3: 1, 4: 2, 5: 2},
			missingFrom: 6,
		},
		{
			name:        "lower incoming term replaces unacknowledged suffix",
			physical:    entries(3, 4, 2),
			incoming:    entries(3, 4, 1),
			wantTerms:   map[uint64]uint64{1: 1, 2: 1, 3: 1, 4: 1},
			missingFrom: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wal := openWAL(t)
			logStore, _ := newTestLogStore(t, wal, time.Second)
			require.NoError(t, logStore.StoreLogs(entries(1, 2, 1)))
			require.NoError(t, wal.StoreLogs(tt.physical))
			require.NoError(t, logStore.StoreLogs(tt.incoming))

			for index, term := range tt.wantTerms {
				var got raft.Log
				require.NoError(t, wal.GetLog(index, &got))
				require.Equal(t, index, got.Index)
				require.Equal(t, term, got.Term)
			}
			if tt.missingFrom > 0 {
				var got raft.Log
				require.ErrorIs(t, wal.GetLog(tt.missingFrom, &got), raft.ErrLogNotFound)
			}
		})
	}
}

func TestTimeoutLogStore_WaitingWriteIsNotDispatched(t *testing.T) {
	wal := openWAL(t)
	gated := newGatedLogStore(wal)
	logStore, timeouts := newTestLogStore(t, gated, 75*time.Millisecond)
	gate := gated.arm()
	t.Cleanup(gate.unblock)

	firstErr := make(chan error, 1)
	go func() { firstErr <- logStore.StoreLogs([]*raft.Log{entry(1, 1)}) }()
	waitForGate(t, gate)
	require.ErrorContains(t, <-firstErr, "log store write timed out")
	require.ErrorContains(t, logStore.StoreLogs([]*raft.Log{entry(2, 1)}), "log store write timed out")
	require.Equal(t, 1, gated.writeCount())
	require.Equal(t, float64(2), testutil.ToFloat64(timeouts))

	gate.unblock()
	waitForLastIndex(t, wal, 1)
	require.Equal(t, 1, gated.writeCount())
	require.NoError(t, logStore.StoreLogs([]*raft.Log{entry(2, 1)}))
	waitForLastIndex(t, wal, 2)
}

func TestTimeoutLogStore_DeleteRangeTimesOutWaitingForWrite(t *testing.T) {
	wal := openWAL(t)
	gated := newGatedLogStore(wal)
	logStore, _ := newTestLogStore(t, gated, 75*time.Millisecond)
	require.NoError(t, logStore.StoreLogs(entries(1, 2, 1)))
	gate := gated.arm()
	t.Cleanup(gate.unblock)

	errCh := make(chan error, 1)
	go func() { errCh <- logStore.StoreLogs([]*raft.Log{entry(3, 1)}) }()
	waitForGate(t, gate)
	require.ErrorContains(t, <-errCh, "log store write timed out")
	require.ErrorContains(t, logStore.DeleteRange(1, 2), "timed out waiting for in-flight log store write")
	require.Equal(t, 0, gated.deleteCount())

	gate.unblock()
	waitForLastIndex(t, wal, 3)
	require.NoError(t, logStore.DeleteRange(1, 2))
	require.Equal(t, 1, gated.deleteCount())
	waitForLastIndex(t, wal, 0)
}

func TestTimeoutLogStore_DeleteRange(t *testing.T) {
	tests := []struct {
		name                string
		acknowledgedLast    uint64
		physicalLast        uint64
		min, max            uint64
		wantFirst, wantLast uint64
	}{
		{
			name:             "logical suffix includes physical tail",
			acknowledgedLast: 20,
			physicalLast:     30,
			min:              15,
			max:              20,
			wantFirst:        1,
			wantLast:         14,
		},
		{
			name:             "prefix compaction preserves tail",
			acknowledgedLast: 30,
			physicalLast:     30,
			min:              1,
			max:              10,
			wantFirst:        11,
			wantLast:         30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wal := openWAL(t)
			logStore, _ := newTestLogStore(t, wal, time.Second)
			require.NoError(t, logStore.StoreLogs(entries(1, tt.acknowledgedLast, 1)))
			if tt.physicalLast > tt.acknowledgedLast {
				require.NoError(t, wal.StoreLogs(entries(tt.acknowledgedLast+1, tt.physicalLast, 1)))
			}
			require.NoError(t, logStore.DeleteRange(tt.min, tt.max))
			first, err := wal.FirstIndex()
			require.NoError(t, err)
			require.Equal(t, tt.wantFirst, first)
			last, err := wal.LastIndex()
			require.NoError(t, err)
			require.Equal(t, tt.wantLast, last)
		})
	}
}
