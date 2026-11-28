package raftnode

import (
	"flag"
	"testing"
	"testing/synctest"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/raft"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

func TestSlowWALTransport_Threshold(t *testing.T) {
	for _, tc := range []struct {
		name    string
		fast    int
		slow    int
		latency float64
		want    bool
	}{
		{name: "empty"},
		{name: "at threshold", slow: 1, latency: 0.1},
		{name: "above threshold without a sample gate", slow: 1, latency: 0.100001, want: true},
		{name: "isolated outlier", fast: 1999, slow: 1, latency: 1},
		{name: "slow tail", fast: 1970, slow: 30, latency: 0.2, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			latency := newMetrics(nil).logStoreWriteRecent
			for range tc.fast {
				latency.Observe(0.01)
			}
			for range tc.slow {
				latency.Observe(tc.latency)
			}
			transport := &slowWALTransport{latency: latency}
			got, err := transport.delayCampaign(2)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestSlowWALTransport_Penalty(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		latency := newMetrics(nil).logStoreWriteRecent
		latency.Observe(0.2)
		transport := &slowWALTransport{latency: latency}
		check := func(term uint64, want bool) {
			t.Helper()
			got, err := transport.delayCampaign(term)
			require.NoError(t, err)
			require.Equal(t, want, got)
		}
		check(2, true)
		deadline := transport.delayUntil
		time.Sleep(slowWALCampaignDelay - time.Nanosecond)
		check(2, true)
		time.Sleep(time.Nanosecond)
		check(2, false)
		require.Equal(t, deadline, transport.delayUntil, "retries must not renew the penalty")
		check(3, true)
		deadline = transport.delayUntil
		check(2, true)
		require.Equal(t, deadline, transport.delayUntil, "a late request from an older term must not renew the penalty")

		// Recovery releases the penalty before its deadline.
		for range 2000 {
			latency.Observe(0.01)
		}
		check(3, false)
		require.True(t, time.Now().Before(deadline))
	})
}

func TestSlowWALTransport_RollingExpiration(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		latency := newMetrics(nil).logStoreWriteRecent
		transport := &slowWALTransport{latency: latency}
		time.Sleep(59 * time.Second)
		latency.Observe(0.2)
		time.Sleep(4 * time.Minute)
		delayed, err := transport.delayCampaign(2)
		require.NoError(t, err)
		require.True(t, delayed)

		// Summary expires age buckets, so this observation expires at minute five.
		time.Sleep(2 * time.Second)
		delayed, err = transport.delayCampaign(2)
		require.NoError(t, err)
		require.False(t, delayed, "expired observations must release the penalty")
		var metric dto.Metric
		require.NoError(t, latency.Write(&metric))
		require.Equal(t, uint64(1), metric.GetSummary().GetSampleCount(), "the cumulative count is not an election gate")
	})
}

func TestNode_WALObservations(t *testing.T) {
	var cfg Config
	cfg.RegisterFlagsWithPrefix("", flag.NewFlagSet("test", flag.ContinueOnError))
	cfg.Dir = t.TempDir()
	cfg.SnapshotsDir = cfg.Dir
	cfg.LogStoreTimeout = 50 * time.Millisecond
	var gated *gatedLogStore
	cfg.LogStoreMiddleware = func(store raft.LogStore) raft.LogStore {
		gated = newGatedLogStore(store)
		return gated
	}
	node := &Node{config: cfg, metrics: newMetrics(nil)}
	require.NoError(t, node.openStore())
	t.Cleanup(node.Shutdown)

	require.NoError(t, node.logStore.StoreLogs(entries(1, 10, 1)))
	var histogram, summary dto.Metric
	require.NoError(t, node.metrics.logStoreWriteRecent.Write(&summary))
	require.Equal(t, uint64(1), summary.GetSummary().GetSampleCount(), "one observation per batch, not per entry")

	gate := gated.arm()
	t.Cleanup(func() {
		gate.unblock()
		waitForLogStoreWrite(t, node.logStore.(*timeoutLogStore))
	})
	require.ErrorContains(t, node.logStore.StoreLogs(entries(11, 20, 1)), "timed out")
	require.NoError(t, node.metrics.logStoreWrite.Write(&histogram))
	require.NoError(t, node.metrics.logStoreWriteRecent.Write(&summary))
	require.Equal(t, uint64(2), summary.GetSummary().GetSampleCount())
	require.Equal(t, histogram.GetHistogram().GetSampleCount(), summary.GetSummary().GetSampleCount())
	require.Equal(t, histogram.GetHistogram().GetSampleSum(), summary.GetSummary().GetSampleSum())
}

func TestSlowWALTransport_VotingAndLeadershipTransfer(t *testing.T) {
	t.Parallel()
	const size = 5
	nodes, transports := newElectionCluster(t, size, false)
	leader := electionLeader(t, nodes)
	future := nodes[leader].Apply([]byte("before election"), time.Second)
	require.NoError(t, future.Error())
	for _, node := range nodes {
		require.Eventually(t, func() bool { return node.AppliedIndex() >= future.Index() }, 3*time.Second, 10*time.Millisecond)
	}

	healthy := (leader + 1) % size
	for i := range nodes {
		if i != leader && i != healthy {
			transports[i].latency.Observe(0.2)
		}
	}
	// Let the slow followers reach pre-vote before the healthy candidate.
	cfg := nodes[healthy].ReloadableConfig()
	cfg.HeartbeatTimeout = 500 * time.Millisecond
	cfg.ElectionTimeout = 500 * time.Millisecond
	require.NoError(t, nodes[healthy].ReloadConfig(cfg))
	require.NoError(t, nodes[leader].Shutdown().Error())
	slow := (healthy + 1) % size
	require.Eventually(t, func() bool {
		transports[slow].mu.Lock()
		defer transports[slow].mu.Unlock()
		return !transports[slow].delayUntil.IsZero()
	}, 2*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return nodes[healthy].State() == raft.Leader }, 3*time.Second, 10*time.Millisecond)
	transports[slow].mu.Lock()
	deadline := transports[slow].delayUntil
	transports[slow].mu.Unlock()
	require.True(t, time.Now().Before(deadline), "the election needs slow peers' votes before their penalty expires")
	require.NoError(t, nodes[healthy].Apply([]byte("after election"), time.Second).Error())

	// Explicit transfer can select a slow replica while its penalty is active.
	addr := transports[slow].LocalAddr()
	require.NoError(t, nodes[healthy].LeadershipTransferToServer(raft.ServerID(addr), addr).Error())
	require.Eventually(t, func() bool { return nodes[slow].State() == raft.Leader }, time.Second, 10*time.Millisecond)
	require.True(t, time.Now().Before(deadline))
	require.NoError(t, nodes[slow].Apply([]byte("transferred"), time.Second).Error())
}

func TestSlowWALTransport_AllSlowCanElect(t *testing.T) {
	t.Parallel()
	start := time.Now()
	nodes, _ := newElectionCluster(t, 3, true)
	leader := electionLeader(t, nodes)
	require.GreaterOrEqual(t, time.Since(start), slowWALCampaignDelay)
	require.NoError(t, nodes[leader].Apply([]byte("all slow"), time.Second).Error())
	require.Equal(t, 100*time.Millisecond, nodes[leader].ReloadableConfig().HeartbeatTimeout)
	require.Never(t, func() bool { return nodes[leader].State() != raft.Leader }, time.Second, 10*time.Millisecond)
	require.NoError(t, nodes[leader].VerifyLeader().Error())
}

func newElectionCluster(t *testing.T, size int, slow bool) ([]*raft.Raft, []*slowWALTransport) {
	t.Helper()
	var nodes []*raft.Raft
	var transports []*slowWALTransport
	var configuration raft.Configuration
	t.Cleanup(func() {
		for _, node := range nodes {
			require.NoError(t, node.Shutdown().Error())
		}
		for _, transport := range transports {
			require.NoError(t, transport.Close())
		}
	})
	for range size {
		network, err := raft.NewTCPTransportWithConfig("127.0.0.1:0", nil, &raft.NetworkTransportConfig{
			MaxPool: 3, Timeout: time.Second, MaxRPCsInFlight: raftMaxRPCsInFlight, Logger: hclog.NewNullLogger(),
		})
		require.NoError(t, err)
		transport := &slowWALTransport{NetworkTransport: network, latency: newMetrics(nil).logStoreWriteRecent}
		if slow {
			transport.latency.Observe(0.2)
		}
		transports = append(transports, transport)
		configuration.Servers = append(configuration.Servers, raft.Server{
			ID: raft.ServerID(network.LocalAddr()), Address: network.LocalAddr(), Suffrage: raft.Voter,
		})
	}
	for _, transport := range transports {
		cfg := raft.DefaultConfig()
		cfg.LocalID = raft.ServerID(transport.LocalAddr())
		cfg.HeartbeatTimeout = 100 * time.Millisecond
		cfg.ElectionTimeout = 100 * time.Millisecond
		cfg.LeaderLeaseTimeout = 50 * time.Millisecond
		cfg.Logger = hclog.NewNullLogger()
		store := raft.NewInmemStore()
		snapshots := raft.NewInmemSnapshotStore()
		require.NoError(t, raft.BootstrapCluster(cfg, store, store, snapshots, transport, configuration))
		node, err := raft.NewRaft(cfg, new(raft.MockFSM), store, store, snapshots, transport)
		require.NoError(t, err)
		nodes = append(nodes, node)
	}
	return nodes, transports
}

func electionLeader(t *testing.T, nodes []*raft.Raft) int {
	t.Helper()
	leader := -1
	require.Eventually(t, func() bool {
		for i, node := range nodes {
			if node.State() == raft.Leader {
				leader = i
				return true
			}
		}
		return false
	}, 10*time.Second, 10*time.Millisecond)
	return leader
}
