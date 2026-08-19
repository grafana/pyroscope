package raftnode

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

// blockingLogStore models a disk that has stopped answering: StoreLogs never
// returns until the test releases it, which is what drives withTimeout past
// its deadline.
type blockingLogStore struct {
	raft.LogStore
	release chan struct{}
}

func (b *blockingLogStore) StoreLogs([]*raft.Log) error {
	<-b.release
	return nil
}

func TestTimeoutLogStore_ExitOnTimeout(t *testing.T) {
	for _, tc := range []struct {
		name       string
		exitOn     bool
		wantExited bool
	}{
		{name: "disabled returns an error to raft", exitOn: false, wantExited: false},
		{name: "enabled terminates the process", exitOn: true, wantExited: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			blocking := &blockingLogStore{release: make(chan struct{})}
			// Released only at the end: the write is abandoned, not
			// cancelled, so the goroutine is still parked in StoreLogs when
			// withTimeout gives up on it.
			t.Cleanup(func() { close(blocking.release) })

			var exited atomic.Bool
			cfg := Config{
				LogStoreTimeout:       50 * time.Millisecond,
				ExitOnLogStoreTimeout: tc.exitOn,
			}
			s := newTimeoutLogStore(blocking, cfg, newMetrics(prometheus.NewRegistry()), nil,
				func(int) { exited.Store(true) })

			err := s.StoreLogs([]*raft.Log{{Index: 1, Term: 1}})

			// The error is returned either way; with the flag on, the real
			// os.Exit would not have come back at all.
			require.ErrorContains(t, err, "timed out")
			require.Equal(t, tc.wantExited, exited.Load())
		})
	}
}

func TestTimeoutLogStore_NoExitOnSuccessfulWrite(t *testing.T) {
	var exited atomic.Bool
	cfg := Config{
		LogStoreTimeout:       time.Minute,
		ExitOnLogStoreTimeout: true,
	}
	s := newTimeoutLogStore(raft.NewInmemStore(), cfg, newMetrics(prometheus.NewRegistry()), nil,
		func(int) { exited.Store(true) })

	require.NoError(t, s.StoreLogs([]*raft.Log{{Index: 1, Term: 1}}))
	require.False(t, exited.Load())
}
