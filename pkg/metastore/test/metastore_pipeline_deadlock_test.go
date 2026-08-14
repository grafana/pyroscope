package test

import (
	"context"
	"crypto/rand"
	"fmt"
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

// TestMetastoreRaftPipelineDeadlock reproduces the permanent replication
// stall observed in the Azure dev cells, where a single metastore replica
// stops receiving log entries entirely and never recovers without a manual
// pod restart.
//
// The observed production signature is:
//
//   - the affected pod is Running, has never restarted, and reports
//     raft_state=Follower;
//   - it is a Voter in the leader's configuration and heartbeats reach it
//     (it never wins or triggers an election);
//   - its raft_log_store_write_duration_seconds_count rate is exactly zero
//     while healthy peers sustain ~35 writes/s, and its snapshot restore
//     count never increases, so it is receiving neither AppendEntries nor
//     InstallSnapshot;
//   - /ready therefore fails forever with "replica has fallen too far
//     behind" (raftnode.ErrLagBehind), and the StatefulSet never converges.
//
// The cause is a deadlock in hashicorp/raft's pipeline replication, not in
// Pyroscope code. In raft v1.7.3 the network transport is constructed with
// MaxRPCsInFlight left at DefaultMaxRPCsInFlight (2), which makes both
// netPipeline channels unbuffered:
//
//	inprogressCh: make(chan *appendFuture, maxInFlight-2)  // cap 0
//	doneCh:       make(chan AppendFuture, maxInFlight-2)   // cap 0
//
// Given a follower that stops answering AppendEntries for longer than the
// transport timeout, three goroutines interlock:
//
//  1. netPipeline.decodeResponses takes future #1 off inprogressCh and
//     blocks decoding its response under a read deadline.
//  2. pipelineSend issues future #2; the handoff blocks on the unbuffered
//     inprogressCh because decodeResponses is still busy with #1.
//  3. The read deadline fires, #1 is failed and handed to doneCh, where
//     Raft.pipelineDecode picks it up. resp.Success is false, so
//     pipelineDecode returns and closes finishCh — the only reader of
//     doneCh is now gone.
//  4. decodeResponses loops, takes #2 off inprogressCh (unblocking the
//     sender), and blocks decoding it.
//  5. The SEND loop in pipelineReplicate now selects between the closed
//     finishCh and a ready triggerCh. If it picks triggerCh it issues
//     future #3, which blocks on inprogressCh.
//  6. #2's read deadline fires; decodeResponses blocks forever on
//     "doneCh <- future" because pipelineDecode has exited.
//
// pipeline.Close is deferred inside pipelineReplicate, which is itself
// parked in step 5, so shutdownCh is never closed and neither goroutine can
// escape. replicate() never returns, so its "defer close(stopHeartbeat)"
// never runs: heartbeats to that peer continue forever, which is precisely
// why the victim stays a quiet Follower instead of timing out and forcing
// an election that would have healed the cluster.
//
// Step 5 is a 50/50 select race, so the test drives several stall/recover
// rounds. Each round needs the peer healthy first, because raft only
// re-enters pipeline mode after a successful replicateTo sets allowPipeline.
//
// The test asserts the behaviour we want — that a follower which was
// briefly stalled always catches up again — so it FAILS on the deadlock.
func TestMetastoreRaftPipelineDeadlock(t *testing.T) {
	const (
		clusterSize = 3
		writers     = 8
		rounds      = 20
		stallFor    = 2 * time.Second
		// Generous relative to how fast a healthy follower catches up under
		// this write load (well under a second), so failing it means
		// "never", not "slow". Permanence is then confirmed separately over
		// a much longer window.
		recoveryBudget  = 10 * time.Second
		permanenceCheck = 45 * time.Second
	)

	stalls := make([]*pipelineStallInjector, clusterSize)
	for i := range stalls {
		stalls[i] = &pipelineStallInjector{}
	}

	cfg := new(metastore.Config)
	flagext.DefaultValues(cfg)
	// The two timeouts must be ordered log store < transport, which is what
	// makes the fault deterministic. In production both default to 10s, so
	// which one fires first is a race — one reason this surfaces every few
	// weeks rather than every time a disk hiccups.
	//
	// Log store first: the stalled follower's own timeoutLogStore (added in
	// #4892) fails StoreLogs, and raft's appendEntries handler returns
	// without setting Success, so the leader gets a prompt Success=false
	// over a still-open connection. That is what makes pipelineDecode exit
	// in step 3 while sends keep flowing.
	//
	// If the transport read deadline fired first instead, decodeResponse
	// would call conn.Release() on the way out, and the next pipelineSend
	// would fail fast on a closed connection and unwind the pipeline
	// cleanly — the leader escapes, and the bug stays hidden.
	cfg.Raft.LogStoreTimeout = 200 * time.Millisecond
	cfg.Raft.TransportTimeout = 5 * time.Second

	var mu sync.Mutex
	nodeIdx := 0
	cfg.Raft.LogStoreMiddleware = func(store raft.LogStore) raft.LogStore {
		mu.Lock()
		i := nodeIdx
		nodeIdx++
		mu.Unlock()
		stalls[i].store = store
		return stalls[i]
	}

	ms := NewMetastoreSet(t, cfg, clusterSize, memory.NewInMemBucket())
	defer func() {
		for _, s := range stalls {
			s.release()
		}
		// Shutting down a raft node whose replication goroutine is wedged
		// blocks forever, which would turn a reproduced failure into an
		// unhelpful test binary timeout. Give teardown a bounded window and
		// let the process exit if it cannot finish.
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

	// Warm up: sustained writes push every follower through a successful
	// replicateTo, which sets allowPipeline and moves replication into
	// pipeline mode. Without this the leader is still in RPC mode when we
	// stall, and the pipeline code under test never runs.
	writer.start()
	requireFollowerCaughtUp(t, ms, leaderIdx, followerIdx, 20*time.Second)
	t.Log("cluster healthy, replication is in pipeline mode")

	for round := 1; round <= rounds; round++ {
		stalls[followerIdx].stall()
		// Hold the stall long enough for many AppendEntries to fail on the
		// follower, so the leader reliably works through steps 1-4 above and
		// gets its chance at the step 5 race.
		time.Sleep(stallFor)
		stalls[followerIdx].release()

		// The disk is healthy again and writes keep flowing, so a follower
		// that is merely behind will now catch up within seconds. One that
		// the leader has stopped replicating to never will.
		recovered, dump := awaitRecovery(t, ms, leaderIdx, followerIdx, recoveryBudget)
		if recovered {
			t.Logf("round %d: pipeline exited cleanly and the follower recovered", round)
			continue
		}

		logLag(t, ms, leaderIdx, followerIdx,
			fmt.Sprintf("round %d: follower cut off with a healthy disk", round))

		// Confirm this is the production failure mode and not a slow
		// recovery: the disk has been healthy for a while now, so give it
		// far longer than a healthy follower could possibly need.
		recovered, _ = awaitRecovery(t, ms, leaderIdx, followerIdx, permanenceCheck)
		require.False(t, recovered, "follower unexpectedly recovered late")
		logLag(t, ms, leaderIdx, followerIdx, fmt.Sprintf(
			"after a further %s of healthy disk and continuous writes", permanenceCheck))

		t.Logf("full goroutine dump written to %s", writeGoroutineDump(t))
		require.NotEmpty(t, dump,
			"the follower stopped replicating, but the raft pipeline goroutines "+
				"are not in the expected interlock — see the dump above, the "+
				"stall has some other cause")
		t.Logf("raft pipeline replication is deadlocked:\n%s", dump)

		t.Fatalf("reproduced: follower never recovers from a %s disk stall; "+
			"the leader has stopped replicating to it entirely", stallFor)
	}

	t.Skipf("pipeline deadlock did not trigger in %d rounds; step 5 is a select "+
		"race, re-run or raise the round count", rounds)
}

func logLag(t *testing.T, ms MetastoreSet, leaderIdx, followerIdx int, what string) {
	t.Helper()
	leader := pipelineNodeInfo(t, ms, leaderIdx)
	follower := pipelineNodeInfo(t, ms, followerIdx)
	t.Logf("%s: leader commit=%d, follower applied=%d, lag=%d entries",
		what, leader.GetCommitIndex(), follower.GetAppliedIndex(),
		leader.GetCommitIndex()-follower.GetAppliedIndex())
}

// writeGoroutineDump saves every goroutine stack so the interlock can be
// inspected directly. t.TempDir is removed on success, and this only runs on
// failure, so the file survives for post-mortem.
func writeGoroutineDump(t *testing.T) string {
	t.Helper()
	buf := make([]byte, 1<<24)
	buf = buf[:runtime.Stack(buf, true)]
	path := filepath.Join(os.TempDir(), "metastore-pipeline-deadlock.stacks")
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Logf("could not write goroutine dump: %v", err)
		return "<unwritten>"
	}
	return path
}

// awaitRecovery waits for the follower to catch up, sampling the pipeline
// goroutines as it goes. If it never catches up, the returned dump holds the
// deadlocked stacks (empty if the interlock was not present, which would mean
// the stall has a different cause and the reproduction is not valid).
func awaitRecovery(t *testing.T, ms MetastoreSet, leaderIdx, followerIdx int, within time.Duration) (bool, string) {
	t.Helper()
	var dump string
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if followerCaughtUp(t, ms, leaderIdx, followerIdx) {
			return true, ""
		}
		if dump == "" {
			if deadlocked, d := pipelineDeadlockStacks(); deadlocked {
				dump = d
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false, dump
}

// followerCaughtUp reports whether the follower's applied index has reached
// the leader's commit index. This is the same condition Metastore.CheckReady
// enforces, so it is what decides pod readiness in a real cell.
func followerCaughtUp(t *testing.T, ms MetastoreSet, leaderIdx, followerIdx int) bool {
	t.Helper()
	leader := pipelineNodeInfo(t, ms, leaderIdx)
	follower := pipelineNodeInfo(t, ms, followerIdx)
	if leader == nil || follower == nil {
		return false
	}
	return follower.GetAppliedIndex() >= leader.GetCommitIndex()
}

func requireFollowerCaughtUp(t *testing.T, ms MetastoreSet, leaderIdx, followerIdx int, within time.Duration) {
	t.Helper()
	require.Eventually(t, func() bool {
		return followerCaughtUp(t, ms, leaderIdx, followerIdx)
	}, within, 250*time.Millisecond, "follower did not catch up with the leader")
}

// pipelineDeadlockStacks looks for the exact interlock described above:
// netPipeline.AppendEntries parked on the inprogressCh handoff while
// netPipeline.decodeResponses is parked on the doneCh handoff to a
// pipelineDecode that has already returned. Requiring both halves keeps
// this from firing on a merely slow pipeline.
//
// Both sites are channel sends inside a select, which the runtime reports
// as "[select]" rather than "[chan send]", so the frames are what identify
// them. A healthy decodeResponses is distinguishable because it is inside
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

// blockWriter keeps a steady stream of concurrent AddBlock calls going
// through the leader. Sustained traffic is what keeps the leader's
// triggerCh signalled, which is what gives the SEND loop a case to pick
// other than the closed finishCh in step 5. Without real write pressure the
// loop almost always observes finishCh alone, unwinds cleanly, and the bug
// stays hidden.
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

// pipelineStallInjector wraps a raft.LogStore so writes can be held for an
// arbitrary period and then allowed through to the real store. Because the
// follower's appendEntries handler calls StoreLogs synchronously before it
// answers, stalling the store is how a follower is made to stop responding
// to the leader — the same thing a hung disk does in a cell.
type pipelineStallInjector struct {
	store   raft.LogStore
	mu      sync.RWMutex
	stalled chan struct{}
}

func (s *pipelineStallInjector) stall() {
	s.mu.Lock()
	if s.stalled == nil {
		s.stalled = make(chan struct{})
	}
	s.mu.Unlock()
}

func (s *pipelineStallInjector) release() {
	s.mu.Lock()
	if s.stalled != nil {
		close(s.stalled)
		s.stalled = nil
	}
	s.mu.Unlock()
}

func (s *pipelineStallInjector) awaitRelease() {
	s.mu.RLock()
	ch := s.stalled
	s.mu.RUnlock()
	if ch != nil {
		<-ch
	}
}

func (s *pipelineStallInjector) FirstIndex() (uint64, error) { return s.store.FirstIndex() }
func (s *pipelineStallInjector) LastIndex() (uint64, error)  { return s.store.LastIndex() }

func (s *pipelineStallInjector) GetLog(idx uint64, log *raft.Log) error {
	return s.store.GetLog(idx, log)
}

func (s *pipelineStallInjector) DeleteRange(min, max uint64) error {
	return s.store.DeleteRange(min, max)
}

func (s *pipelineStallInjector) IsMonotonic() bool {
	if m, ok := s.store.(raft.MonotonicLogStore); ok {
		return m.IsMonotonic()
	}
	return false
}

func (s *pipelineStallInjector) StoreLog(log *raft.Log) error {
	return s.StoreLogs([]*raft.Log{log})
}

func (s *pipelineStallInjector) StoreLogs(logs []*raft.Log) error {
	s.awaitRelease()
	return s.store.StoreLogs(logs)
}
