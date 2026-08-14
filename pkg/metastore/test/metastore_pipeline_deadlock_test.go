package test

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/dskit/flagext"
	"github.com/hashicorp/raft"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/metastore"
	"github.com/grafana/pyroscope/v2/pkg/metastore/raftnode/raftnodepb"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/memory"
)

// TestRaftPipelineDeadlock covers the workaround pinned by
// raftMaxRPCsInFlight: on raft v1.7.3, a brief log store stall on a follower
// can deadlock the leader's pipeline replication to that follower forever.
//
// In raft v1.7.3 the network transport sizes both netPipeline channels at
// MaxRPCsInFlight-2, so the default of 2 leaves them unbuffered. Given a
// follower that fails an AppendEntries, five things happen in order:
//
//  1. decodeResponses takes future #1 off inprogressCh and blocks decoding.
//  2. pipelineSend issues #2, which blocks handing off to inprogressCh.
//  3. #1's response comes back with Success=false, so Raft.pipelineDecode
//     returns and closes finishCh. doneCh now has no reader.
//  4. decodeResponses loops, takes #2 (unblocking the sender), and blocks
//     decoding it.
//  5. The SEND loop selects between the closed finishCh and a ready
//     triggerCh. If it picks triggerCh it issues #3, which parks on
//     inprogressCh; #2's response then parks decodeResponses on doneCh.
//
// pipeline.Close is deferred inside pipelineReplicate, which is stuck in step
// 5, so shutdownCh never closes and neither goroutine escapes. replicate()
// never returns either, so its deferred close(stopHeartbeat) never runs:
// heartbeats to the victim keep flowing, it stays a quiet Follower that never
// forces an election, and it is simply never replicated to again. The stalled
// replica then fails readiness on ErrLagBehind indefinitely. Restarting it
// does not help, because the wedge is on the leader.
//
// Step 5 is a select race, so the fault is applied over several rounds.
// Measured at roughly nine times in ten per round; each round needs the peer
// healthy first, since raft only re-enters pipeline mode after a successful
// replicateTo sets allowPipeline. Hence a fault that rejects writes rather
// than delaying them — see failInjector.
//
// The assertion is that the follower catches up again once its log store is
// healthy, which is what readiness depends on. The goroutine stacks are only
// consulted to explain a failure.
func TestRaftPipelineDeadlock(t *testing.T) {
	const (
		clusterSize = 3
		writers     = 8
		rounds      = 4
		failFor     = time.Second
		recoverIn   = 15 * time.Second
		// Mirrors a raft.log-store-timeout well under raft.transport-timeout,
		// which is the ordering that produces this failure in production.
		failAfter = 200 * time.Millisecond
	)

	faults := make([]*failInjector, clusterSize)
	for i := range faults {
		faults[i] = &failInjector{delay: failAfter}
	}

	cfg := new(metastore.Config)
	flagext.DefaultValues(cfg)

	var mu sync.Mutex
	nodeIdx := 0
	cfg.Raft.LogStoreMiddleware = func(store raft.LogStore) raft.LogStore {
		mu.Lock()
		i := nodeIdx
		nodeIdx++
		mu.Unlock()
		faults[i].store = store
		return faults[i]
	}

	ms := NewMetastoreSet(t, cfg, clusterSize, memory.NewInMemBucket())
	defer func() {
		for _, f := range faults {
			f.clear()
		}
		// Shutting down a raft node whose replication goroutine is wedged
		// blocks forever, which would turn a reproduced failure into an
		// unhelpful test binary timeout.
		done := make(chan struct{})
		go func() {
			defer close(done)
			ms.Close()
		}()
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			t.Log("teardown timed out; raft shutdown is blocked on the wedged node")
		}
	}()

	leaderIdx := findLeader(t, ms)
	followerIdx := (leaderIdx + 1) % clusterSize
	t.Logf("leader=node-%d victim follower=node-%d", leaderIdx, followerIdx)

	writer := newBlockWriter(ms, leaderIdx, writers)
	defer writer.stop()

	// Sustained writes push every follower through a successful replicateTo,
	// which sets allowPipeline and moves replication into pipeline mode.
	// Without this the leader is still in RPC mode when we stall, and the
	// code under test never runs.
	writer.start()
	require.Eventually(t, func() bool {
		return followerCaughtUp(t, ms, leaderIdx, followerIdx)
	}, 20*time.Second, 250*time.Millisecond, "follower did not catch up with the leader")
	t.Log("cluster healthy; replication would be pipelining if it were enabled")

	for round := 1; round <= rounds; round++ {
		faults[followerIdx].fail()
		time.Sleep(failFor)
		faults[followerIdx].clear()

		// The follower's log store is healthy again and writes keep flowing,
		// so a follower that is merely behind catches up in well under a
		// second. One the leader has stopped replicating to never will.
		if !awaitCatchUp(t, ms, leaderIdx, followerIdx, recoverIn) {
			if deadlocked, dump := pipelineDeadlockStacks(); deadlocked {
				t.Logf("round %d: raft pipeline replication is deadlocked\n%s", round, dump)
			} else {
				t.Logf("round %d: follower is not catching up, but the raft pipeline "+
					"goroutines are not in the expected interlock", round)
			}
			t.Logf("full goroutine dump written to %s", writeGoroutineDump(t))
			logLag(t, ms, leaderIdx, followerIdx, "victim")
			t.Fatal("the leader has permanently stopped replicating to the follower")
		}
	}
}

// awaitCatchUp waits for the follower's applied index to reach the leader's
// commit index, which is the same condition Metastore.CheckReady enforces and
// therefore what decides pod readiness in a real cell.
func awaitCatchUp(t *testing.T, ms MetastoreSet, leaderIdx, followerIdx int, within time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if followerCaughtUp(t, ms, leaderIdx, followerIdx) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func followerCaughtUp(t *testing.T, ms MetastoreSet, leaderIdx, followerIdx int) bool {
	t.Helper()
	leader := pipelineNodeInfo(t, ms, leaderIdx)
	follower := pipelineNodeInfo(t, ms, followerIdx)
	if leader == nil || follower == nil {
		return false
	}
	return follower.GetAppliedIndex() >= leader.GetCommitIndex()
}

func logLag(t *testing.T, ms MetastoreSet, leaderIdx, followerIdx int, what string) {
	t.Helper()
	leader := pipelineNodeInfo(t, ms, leaderIdx)
	follower := pipelineNodeInfo(t, ms, followerIdx)
	t.Logf("%s: leader commit=%d, follower applied=%d, lag=%d entries",
		what, leader.GetCommitIndex(), follower.GetAppliedIndex(),
		leader.GetCommitIndex()-follower.GetAppliedIndex())
}

func pipelineNodeInfo(t *testing.T, ms MetastoreSet, idx int) *raftnodepb.NodeInfo {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	resp, err := ms.Instances[idx].NodeInfo(ctx, &raftnodepb.NodeInfoRequest{})
	if err != nil {
		return nil
	}
	return resp.GetNode()
}

// pipelineDeadlockStacks looks for the interlock described above:
// netPipeline.AppendEntries parked on the inprogressCh handoff while
// netPipeline.decodeResponses is parked on the doneCh handoff to a
// pipelineDecode that has already returned. Requiring both halves keeps this
// from firing on a merely slow pipeline.
//
// Both sites are channel sends inside a select, which the runtime reports as
// "[select]" rather than "[chan send]", so the frames are what identify them.
// A healthy decodeResponses is distinguishable because it sits inside
// decodeResponse reading from the connection, not in its own select.
func pipelineDeadlockStacks() (bool, string) {
	buf := make([]byte, 1<<24)
	buf = buf[:runtime.Stack(buf, true)]

	var sendBlocked, decodeBlocked []string
	for g := range strings.SplitSeq(string(buf), "\n\n") {
		if !strings.Contains(g, "[select]") {
			continue
		}
		switch {
		case strings.Contains(g, "netPipeline).AppendEntries"):
			sendBlocked = append(sendBlocked, g)
		case strings.Contains(g, "netPipeline).decodeResponses") &&
			!strings.Contains(g, "raft.decodeResponse("):
			decodeBlocked = append(decodeBlocked, g)
		}
	}
	if len(sendBlocked) == 0 || len(decodeBlocked) == 0 {
		return false, ""
	}
	return true, strings.Join(append(sendBlocked, decodeBlocked...), "\n\n")
}

func writeGoroutineDump(t *testing.T) string {
	t.Helper()
	buf := make([]byte, 1<<24)
	buf = buf[:runtime.Stack(buf, true)]
	path := filepath.Join(t.TempDir(), "goroutines.txt")
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Logf("could not write goroutine dump: %v", err)
		return "<unwritten>"
	}
	return path
}

// blockWriter keeps a steady stream of concurrent AddBlock calls going
// through the leader.
type blockWriter struct {
	ms        MetastoreSet
	leaderIdx int
	writers   int
	seq       atomic.Uint64
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

func newBlockWriter(ms MetastoreSet, leaderIdx, writers int) *blockWriter {
	return &blockWriter{ms: ms, leaderIdx: leaderIdx, writers: writers}
}

func (w *blockWriter) start() {
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	for i := 0; i < w.writers; i++ {
		w.wg.Go(func() {
			for ctx.Err() == nil {
				callCtx, callCancel := context.WithTimeout(ctx, 5*time.Second)
				_, _ = w.ms.Instances[w.leaderIdx].AddBlock(callCtx, &metastorev1.AddBlockRequest{
					Block: &metastorev1.BlockMeta{
						Id: ulid.MustNew(w.seq.Add(1), rand.Reader).String(),
					},
				})
				callCancel()
			}
		})
	}
}

func (w *blockWriter) stop() {
	if w.cancel == nil {
		return
	}
	w.cancel()
	w.wg.Wait()
	w.cancel = nil
}

// failInjector wraps a raft.LogStore so writes can be made slow and then
// rejected, without ever touching the underlying store.
//
// Both halves matter, and for different reasons.
//
// Rejecting rather than eventually writing keeps the follower's log
// consistent, so it catches up as soon as the fault clears and each round
// starts from a healthy peer. A fault that merely delays the write would land
// it out of band and livelock the follower on "non-monotonic log entries" —
// a different bug (#5307) that would both mask this one and make every round
// after the first meaningless.
//
// The delay is what actually creates the deadlock window. It has to be long
// enough that decodeResponses is still busy with one response when the next
// send arrives, so that send parks on inprogressCh; failing instantly instead
// lets every handoff complete and the interlock never forms. In production
// that delay is supplied by timeoutLogStore, which fails a stalled write after
// raft.log-store-timeout. It must stay well below the transport timeout: if
// the leader's read deadline fired first, decodeResponse would call
// conn.Release() and the next send would fail fast on a closed connection,
// letting the leader unwind cleanly.
type failInjector struct {
	store   raft.LogStore
	failing atomic.Bool
	delay   time.Duration
}

func (f *failInjector) fail()  { f.failing.Store(true) }
func (f *failInjector) clear() { f.failing.Store(false) }

func (f *failInjector) FirstIndex() (uint64, error)            { return f.store.FirstIndex() }
func (f *failInjector) LastIndex() (uint64, error)             { return f.store.LastIndex() }
func (f *failInjector) GetLog(idx uint64, log *raft.Log) error { return f.store.GetLog(idx, log) }
func (f *failInjector) DeleteRange(min, max uint64) error      { return f.store.DeleteRange(min, max) }

func (f *failInjector) IsMonotonic() bool {
	if m, ok := f.store.(raft.MonotonicLogStore); ok {
		return m.IsMonotonic()
	}
	return false
}

func (f *failInjector) StoreLog(log *raft.Log) error {
	return f.StoreLogs([]*raft.Log{log})
}

func (f *failInjector) StoreLogs(logs []*raft.Log) error {
	if f.failing.Load() {
		time.Sleep(f.delay)
		return errors.New("simulated slow log store failure")
	}
	return f.store.StoreLogs(logs)
}
