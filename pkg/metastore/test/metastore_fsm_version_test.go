package test

import (
	"context"
	"crypto/rand"
	"strconv"
	"testing"
	"time"

	"github.com/grafana/dskit/flagext"
	"github.com/hashicorp/raft"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/metastore"
	"github.com/grafana/pyroscope/v2/pkg/metastore/fsmversion"
	"github.com/grafana/pyroscope/v2/pkg/metastore/raftnode/raftnodepb"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/memory"
)

func fsmVersionTestConfig() *metastore.Config {
	cfg := new(metastore.Config)
	flagext.DefaultValues(cfg)
	cfg.FSMVersion.CheckInterval = 50 * time.Millisecond
	return cfg
}

func frequentSnapshots(cfg *metastore.Config) {
	cfg.Raft.SnapshotInterval = 100 * time.Millisecond
	cfg.Raft.SnapshotThreshold = 1
	cfg.Raft.SnapshotsRetain = 1
}

func supports(v fsmversion.Version) func(*metastore.Config) {
	return func(cfg *metastore.Config) {
		cfg.FSMVersion.TestingSupportedVersion = &v
	}
}

func supporting(versions ...fsmversion.Version) InstanceOption {
	return func(i int, cfg *metastore.Config) {
		supports(versions[i])(cfg)
	}
}

func nodeInfo(ms *MetastoreSet, i int) (*raftnodepb.NodeInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	resp, err := ms.Instances[i].NodeInfo(ctx, &raftnodepb.NodeInfoRequest{})
	if err != nil {
		return nil, err
	}
	return resp.Node, nil
}

func activeVersions(ms *MetastoreSet) map[int]fsmversion.Version {
	versions := make(map[int]fsmversion.Version)
	for i := range ms.Instances {
		if !ms.running[i] {
			continue
		}
		if info, err := nodeInfo(ms, i); err == nil {
			versions[i] = fsmversion.Version(info.ActiveFsmVersion)
		}
	}
	return versions
}

func requireActiveVersion(t *testing.T, ms *MetastoreSet, want fsmversion.Version) {
	t.Helper()
	require.Eventually(t, func() bool {
		var n int
		for i, v := range activeVersions(ms) {
			if v != want {
				t.Logf("instance %d: active FSM version %d, want %d", i, v, want)
				return false
			}
			n++
		}
		return n == countRunning(ms)
	}, 20*time.Second, 50*time.Millisecond, "all running instances must report active FSM version %d", want)
}

func requireActiveVersionStays(t *testing.T, ms *MetastoreSet, want fsmversion.Version, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		for i, v := range activeVersions(ms) {
			require.Equal(t, want, v, "instance %d", i)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func countRunning(ms *MetastoreSet) int {
	var n int
	for _, r := range ms.running {
		if r {
			n++
		}
	}
	return n
}

func leaderIndex(t *testing.T, ms *MetastoreSet) int {
	t.Helper()
	leader := -1
	require.Eventually(t, func() bool {
		for i := range ms.Instances {
			if !ms.running[i] {
				continue
			}
			if info, err := nodeInfo(ms, i); err == nil && info.State == raft.Leader.String() {
				leader = i
				return true
			}
		}
		return false
	}, 20*time.Second, 50*time.Millisecond, "no leader elected")
	return leader
}

func followerIndex(t *testing.T, ms *MetastoreSet) int {
	t.Helper()
	leader := leaderIndex(t, ms)
	for i := range ms.Instances {
		if i != leader && ms.running[i] {
			return i
		}
	}
	t.Fatal("no follower found")
	return -1
}

func requireWritable(t *testing.T, ms *MetastoreSet) {
	t.Helper()
	block := &metastorev1.BlockMeta{Id: ulid.MustNew(ulid.Now(), rand.Reader).String()}
	require.Eventually(t, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, err := ms.Client.AddBlock(ctx, &metastorev1.AddBlockRequest{Block: block})
		return err == nil
	}, 30*time.Second, 100*time.Millisecond, "the cluster must accept writes")
}

func lastSnapshotIndex(ms *MetastoreSet, i int) uint64 {
	info, err := nodeInfo(ms, i)
	if err != nil {
		return 0
	}
	for j, name := range info.Stats.Name {
		if name == "last_snapshot_index" {
			v, _ := strconv.ParseUint(info.Stats.Value[j], 10, 64)
			return v
		}
	}
	return 0
}

func appliedIndex(t *testing.T, ms *MetastoreSet, i int) uint64 {
	t.Helper()
	info, err := nodeInfo(ms, i)
	require.NoError(t, err)
	return info.AppliedIndex
}

func requireSnapshotted(t *testing.T, ms *MetastoreSet, index uint64) {
	t.Helper()
	require.Eventually(t, func() bool {
		for i := range ms.Instances {
			if ms.running[i] && lastSnapshotIndex(ms, i) < index {
				requireWritable(t, ms)
				return false
			}
		}
		return true
	}, 30*time.Second, 100*time.Millisecond, "all running instances must take a snapshot past index %d", index)
}

func TestFSMVersion_ActivatedOnNewCluster(t *testing.T) {
	ms := NewMetastoreSet(t, fsmVersionTestConfig(), 3, memory.NewInMemBucket())
	defer ms.Close()

	requireActiveVersion(t, &ms, fsmversion.Latest)
	for i := range ms.Instances {
		info, err := nodeInfo(&ms, i)
		require.NoError(t, err)
		assert.Equal(t, uint32(fsmversion.Latest), info.SupportedFsmVersion)
	}
}

func TestFSMVersion_RollingUpgrade(t *testing.T) {
	ms := NewMetastoreSet(t, fsmVersionTestConfig(), 3, memory.NewInMemBucket(), supporting(1, 1, 1))
	defer ms.Close()
	requireActiveVersion(t, &ms, 1)

	for i := range ms.Instances {
		ms.RestartInstance(i, supports(3))
		requireWritable(t, &ms)
		if i < len(ms.Instances)-1 {
			requireActiveVersionStays(t, &ms, 1, 500*time.Millisecond)
		}
	}

	requireActiveVersion(t, &ms, 3)
	requireWritable(t, &ms)
}

func TestFSMVersion_ReplicaWithoutVersionSupportBlocksActivation(t *testing.T) {
	ms := NewMetastoreSet(t, fsmVersionTestConfig(), 3, memory.NewInMemBucket(), supporting(2, 2, fsmversion.Unversioned))
	defer ms.Close()

	requireWritable(t, &ms)
	requireActiveVersionStays(t, &ms, fsmversion.Unversioned, time.Second)

	ms.RestartInstance(2, supports(2))
	requireActiveVersion(t, &ms, 2)
}

func TestFSMVersion_UnreachableReplicaBlocksActivation(t *testing.T) {
	ms := NewMetastoreSet(t, fsmVersionTestConfig(), 3, memory.NewInMemBucket(), supporting(1, 1, 1))
	defer ms.Close()
	requireActiveVersion(t, &ms, 1)

	ms.StopInstance(2)
	ms.RestartInstance(0, supports(2))
	ms.RestartInstance(1, supports(2))
	requireWritable(t, &ms)
	requireActiveVersionStays(t, &ms, 1, time.Second)

	ms.Configure(2, supports(2))
	require.NoError(t, ms.StartInstance(2))
	requireActiveVersion(t, &ms, 2)
}

func TestFSMVersion_MaxVersionLimitsActivation(t *testing.T) {
	cfg := fsmVersionTestConfig()
	cfg.FSMVersion.MaxVersion = 2
	ms := NewMetastoreSet(t, cfg, 3, memory.NewInMemBucket(), supporting(3, 3, 3))
	defer ms.Close()

	requireActiveVersion(t, &ms, 2)
	requireActiveVersionStays(t, &ms, 2, 500*time.Millisecond)

	for i := range ms.Instances {
		ms.RestartInstance(i, func(cfg *metastore.Config) { cfg.FSMVersion.MaxVersion = 0 })
	}
	requireActiveVersion(t, &ms, 3)
}

func TestFSMVersion_ActivationDelay(t *testing.T) {
	const delay = 2 * time.Second
	cfg := fsmVersionTestConfig()
	cfg.FSMVersion.ActivationDelay = delay

	start := time.Now()
	ms := NewMetastoreSet(t, cfg, 3, memory.NewInMemBucket(), supporting(2, 2, 2))
	defer ms.Close()

	requireActiveVersion(t, &ms, 2)
	assert.GreaterOrEqual(t, time.Since(start), delay)
}

func TestFSMVersion_ReplicaRefusesToStartWithUnsupportedVersion(t *testing.T) {
	cfg := fsmVersionTestConfig()
	frequentSnapshots(cfg)
	ms := NewMetastoreSet(t, cfg, 3, memory.NewInMemBucket(), supporting(2, 2, 2))
	defer ms.Close()
	requireActiveVersion(t, &ms, 2)

	follower := followerIndex(t, &ms)
	requireSnapshotted(t, &ms, appliedIndex(t, &ms, follower))

	ms.StopInstance(follower)
	ms.Configure(follower, supports(1))
	err := ms.StartInstance(follower)
	require.ErrorContains(t, err, "failed to load any existing snapshots")
	requireWritable(t, &ms)

	ms.Configure(follower, supports(2))
	require.NoError(t, ms.StartInstance(follower))
	requireActiveVersion(t, &ms, 2)
	requireWritable(t, &ms)
}

func TestFSMVersion_WipedReplicaRecoversVersionFromLog(t *testing.T) {
	ms := NewMetastoreSet(t, fsmVersionTestConfig(), 3, memory.NewInMemBucket(), supporting(2, 2, 2))
	defer ms.Close()
	requireActiveVersion(t, &ms, 2)

	ms.StopInstance(0)
	ms.WipeInstance(0)
	require.NoError(t, ms.StartInstance(0))

	requireActiveVersion(t, &ms, 2)
	requireWritable(t, &ms)
}

func TestFSMVersion_WipedReplicaWithOlderBinaryRejectsSnapshot(t *testing.T) {
	cfg := fsmVersionTestConfig()
	frequentSnapshots(cfg)
	cfg.Raft.TrailingLogs = 0
	ms := NewMetastoreSet(t, cfg, 3, memory.NewInMemBucket(), supporting(2, 2, 2))
	defer ms.Close()
	requireActiveVersion(t, &ms, 2)

	activatedAt := appliedIndex(t, &ms, leaderIndex(t, &ms))
	requireWritable(t, &ms)
	requireSnapshotted(t, &ms, activatedAt+1)

	ms.StopInstance(0)
	ms.WipeInstance(0)
	ms.Configure(0, supports(1))
	require.NoError(t, ms.StartInstance(0))

	requireWritable(t, &ms)
	assert.Never(t, func() bool {
		info, err := nodeInfo(&ms, 0)
		return err == nil && (info.AppliedIndex > 0 || info.ActiveFsmVersion > 0)
	}, 2*time.Second, 100*time.Millisecond, "the replica must not apply a state it does not support")

	ms.RestartInstance(0, supports(2))
	requireActiveVersion(t, &ms, 2)
	requireWritable(t, &ms)
}

func TestFSMVersion_PersistsAcrossLeaderChangeAndRestart(t *testing.T) {
	ms := NewMetastoreSet(t, fsmVersionTestConfig(), 3, memory.NewInMemBucket(), supporting(2, 2, 2))
	defer ms.Close()
	requireActiveVersion(t, &ms, 2)

	disableActivation := func(cfg *metastore.Config) { cfg.FSMVersion.CheckInterval = 0 }
	leader := leaderIndex(t, &ms)
	ms.RestartInstance(leader, disableActivation)
	assert.NotEqual(t, leader, leaderIndex(t, &ms), "leadership must move to another replica")
	requireActiveVersion(t, &ms, 2)

	for i := range ms.Instances {
		if i != leader {
			ms.RestartInstance(i, disableActivation)
		}
	}
	requireWritable(t, &ms)
	requireActiveVersionStays(t, &ms, 2, 500*time.Millisecond)
}
