package fsmversion

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/v2/pkg/metastore/fsm"
)

const recordCommand fsm.RaftLogEntryType = 1000

var recordsBucket = []byte("records")

type record struct {
	Version Version
	Value   string
}

type recorder struct{}

func (recorder) Init(tx *bbolt.Tx) error {
	_, err := tx.CreateBucketIfNotExists(recordsBucket)
	return err
}

func (recorder) Restore(*bbolt.Tx) error { return nil }

func (recorder) record(_ context.Context, tx *bbolt.Tx, cmd *raft.Log, req *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
	value := req.Value
	if IsActive(tx, 2) {
		value = strings.ToUpper(value)
	}
	key := binary.BigEndian.AppendUint64(nil, cmd.Index)
	val := binary.BigEndian.AppendUint32(nil, uint32(Get(tx)))
	return req, tx.Bucket(recordsBucket).Put(key, append(val, value...))
}

type noopRegistry struct{}

func (noopRegistry) Retrieve(string) (context.Context, bool) { return nil, false }
func (noopRegistry) Delete(string)                           {}
func (noopRegistry) Size() int                               { return 0 }

type testNode struct {
	t         *testing.T
	id        raft.ServerID
	addr      raft.ServerAddress
	supported Version
	trailing  uint64

	logs  *raft.InmemStore
	snaps *raft.InmemSnapshotStore
	trans *raft.InmemTransport

	fsm   *fsm.FSM
	state *State
	raft  *raft.Raft
}

type testCluster struct {
	t      *testing.T
	nodes  []*testNode
	active Version
}

func newTestCluster(t *testing.T, trailing uint64, supported ...Version) *testCluster {
	c := &testCluster{t: t}
	var configuration raft.Configuration
	for i, v := range supported {
		n := c.newNode(fmt.Sprintf("node-%d", i), v, trailing)
		configuration.Servers = append(configuration.Servers, raft.Server{ID: n.id, Address: n.addr, Suffrage: raft.Voter})
	}
	for _, n := range c.nodes {
		require.NoError(t, raft.BootstrapCluster(n.raftConfig(), n.logs, n.logs, n.snaps, n.trans, configuration))
		require.NoError(t, n.start())
	}
	t.Cleanup(c.shutdown)
	c.leader()
	return c
}

func (c *testCluster) newNode(id string, supported Version, trailing uint64) *testNode {
	n := &testNode{
		t:         c.t,
		id:        raft.ServerID(id),
		addr:      raft.ServerAddress(id),
		supported: supported,
		trailing:  trailing,
		logs:      raft.NewInmemStore(),
		snaps:     raft.NewInmemSnapshotStore(),
	}
	c.connect(n)
	c.nodes = append(c.nodes, n)
	return n
}

func (c *testCluster) connect(n *testNode) {
	_, n.trans = raft.NewInmemTransport(n.addr)
	for _, other := range c.nodes {
		if other == n {
			continue
		}
		other.trans.Connect(n.addr, n.trans)
		n.trans.Connect(other.addr, other.trans)
	}
}

func (n *testNode) raftConfig() *raft.Config {
	conf := raft.DefaultConfig()
	conf.LocalID = n.id
	conf.HeartbeatTimeout = 50 * time.Millisecond
	conf.ElectionTimeout = 50 * time.Millisecond
	conf.LeaderLeaseTimeout = 50 * time.Millisecond
	conf.CommitTimeout = 5 * time.Millisecond
	conf.SnapshotInterval = time.Hour
	conf.SnapshotThreshold = 1 << 30
	conf.TrailingLogs = n.trailing
	conf.LogOutput = io.Discard
	return conf
}

func (n *testNode) start() error {
	f, err := fsm.New(log.NewNopLogger(), nil, fsm.Config{
		SnapshotCompression: "zstd",
		DataDir:             n.t.TempDir(),
	}, noopRegistry{})
	if err != nil {
		return err
	}
	state := NewState(log.NewNopLogger(), n.supported, nil)
	fsm.RegisterRaftCommandHandler(f, fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_SET_FSM_VERSION), state.SetVersion)
	fsm.RegisterRaftCommandHandler(f, recordCommand, recorder{}.record)
	f.RegisterSnapshotValidator(state)
	f.RegisterRestorer(state, recorder{})

	snapshots, err := n.snaps.List()
	if err != nil {
		f.Shutdown()
		return err
	}
	if len(snapshots) == 0 {
		if err = f.Init(); err != nil {
			f.Shutdown()
			return err
		}
	}
	r, err := raft.NewRaft(n.raftConfig(), f, n.logs, n.logs, n.snaps, n.trans)
	if err != nil {
		f.Shutdown()
		return err
	}
	n.fsm, n.state, n.raft = f, state, r
	return nil
}

func (n *testNode) stop() {
	if n.raft == nil {
		return
	}
	require.NoError(n.t, n.raft.Shutdown().Error())
	n.fsm.Shutdown()
	n.raft = nil
}

func (c *testCluster) restart(n *testNode, supported Version) error {
	n.stop()
	n.supported = supported
	c.connect(n)
	return n.start()
}

func (c *testCluster) shutdown() {
	for _, n := range c.nodes {
		n.stop()
	}
}

func (c *testCluster) leader() *testNode {
	var leader *testNode
	require.Eventually(c.t, func() bool {
		for _, n := range c.nodes {
			if n.raft != nil && n.raft.State() == raft.Leader {
				leader = n
				return true
			}
		}
		return false
	}, 10*time.Second, 10*time.Millisecond, "no leader elected")
	return leader
}

func (c *testCluster) apply(t fsm.RaftLogEntryType, m proto.Message) (proto.Message, uint64) {
	data, err := fsm.MarshalEntry(t, m)
	require.NoError(c.t, err)
	var future raft.ApplyFuture
	require.Eventually(c.t, func() bool {
		future = c.leader().raft.Apply(data, 5*time.Second)
		return future.Error() == nil
	}, 10*time.Second, 10*time.Millisecond)
	resp := future.Response().(fsm.Response)
	require.NoError(c.t, resp.Err)
	return resp.Data, future.Index()
}

func (c *testCluster) record(value string) uint64 {
	_, index := c.apply(recordCommand, wrapperspb.String(value))
	return index
}

func (c *testCluster) setVersion(v Version) uint64 {
	leader := c.leader()
	resp, index := c.apply(
		fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_SET_FSM_VERSION),
		&raft_log.SetFSMVersionRequest{Term: leader.raft.CurrentTerm(), Version: uint32(v)},
	)
	require.Equal(c.t, uint32(v), resp.(*raft_log.SetFSMVersionResponse).Version)
	return index
}

func (c *testCluster) waitConverged(nodes []*testNode, want map[uint64]record) {
	require.Eventually(c.t, func() bool {
		for _, n := range nodes {
			if n.state.Active() != c.active || !assert.ObjectsAreEqual(want, n.records()) {
				return false
			}
		}
		return true
	}, 10*time.Second, 10*time.Millisecond)
}

func (n *testNode) records() map[uint64]record {
	records := make(map[uint64]record)
	require.NoError(n.t, n.fsm.Read(func(tx *bbolt.Tx) {
		_ = tx.Bucket(recordsBucket).ForEach(func(k, v []byte) error {
			records[binary.BigEndian.Uint64(k)] = record{
				Version: Version(binary.BigEndian.Uint32(v[:4])),
				Value:   string(v[4:]),
			}
			return nil
		})
	}))
	return records
}

type step struct {
	version Version
	value   string
}

func (c *testCluster) run(want map[uint64]record, steps []step) {
	for _, s := range steps {
		if s.version > 0 {
			c.setVersion(s.version)
			c.active = s.version
			continue
		}
		index := c.record(s.value)
		value := s.value
		if c.active >= 2 {
			value = strings.ToUpper(value)
		}
		want[index] = record{Version: c.active, Value: value}
	}
}

var gatedSteps = []step{
	{value: "a"}, {value: "b"},
	{version: 1},
	{value: "c"},
	{version: 2},
	{value: "d"}, {value: "e"},
	{version: 3},
	{value: "f"},
}

func TestCluster_GatedBehaviorSwitchesAtTheSameLogIndex(t *testing.T) {
	c := newTestCluster(t, 1024, 3, 3, 3)
	want := make(map[uint64]record)
	c.run(want, gatedSteps)
	c.waitConverged(c.nodes, want)
}

func TestCluster_LogReplayIsDeterministic(t *testing.T) {
	c := newTestCluster(t, 1024, 3, 3, 3)
	want := make(map[uint64]record)
	c.run(want, gatedSteps)
	c.waitConverged(c.nodes, want)

	follower := c.nodes[0]
	if follower == c.leader() {
		follower = c.nodes[1]
	}

	t.Run("full log replay", func(t *testing.T) {
		require.NoError(t, c.restart(follower, 3))
		c.waitConverged([]*testNode{follower}, want)
	})

	t.Run("snapshot and log tail", func(t *testing.T) {
		require.NoError(t, follower.raft.Snapshot().Error())
		c.run(want, []step{{value: "g"}, {value: "h"}})
		c.waitConverged([]*testNode{follower}, want)

		require.NoError(t, c.restart(follower, 3))
		c.waitConverged([]*testNode{follower}, want)
	})
}

func TestCluster_StaleTermProposalIsIgnoredByAllReplicas(t *testing.T) {
	c := newTestCluster(t, 1024, 3, 3, 3)
	want := make(map[uint64]record)
	c.run(want, []step{{version: 1}})

	term := c.leader().raft.CurrentTerm()
	resp, _ := c.apply(
		fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_SET_FSM_VERSION),
		&raft_log.SetFSMVersionRequest{Term: term - 1, Version: 2},
	)
	assert.Equal(t, uint32(1), resp.(*raft_log.SetFSMVersionResponse).Version)

	c.run(want, []step{{value: "a"}})
	c.waitConverged(c.nodes, want)
}

func TestCluster_NewReplicaReceivesVersionWithSnapshot(t *testing.T) {
	c := newTestCluster(t, 0, 3, 3, 3)
	want := make(map[uint64]record)
	c.run(want, gatedSteps)
	c.waitConverged(c.nodes, want)
	leader := c.leader()
	require.NoError(t, leader.raft.Snapshot().Error())

	n := c.newNode("node-3", 3, 0)
	require.NoError(t, n.start())
	require.NoError(t, leader.raft.AddVoter(n.id, n.addr, 0, 5*time.Second).Error())
	c.waitConverged([]*testNode{n}, want)

	snapshots, err := n.snaps.List()
	require.NoError(t, err)
	require.NotEmpty(t, snapshots, "the replica must have been restored from a snapshot")
}

func TestCluster_OlderReplicaRejectsSnapshotWithNewerVersion(t *testing.T) {
	c := newTestCluster(t, 0, 3, 3, 3)
	want := make(map[uint64]record)
	c.run(want, gatedSteps)
	c.waitConverged(c.nodes, want)
	leader := c.leader()
	require.NoError(t, leader.raft.Snapshot().Error())

	n := c.newNode("node-3", 2, 0)
	require.NoError(t, n.start())
	require.NoError(t, leader.raft.AddVoter(n.id, n.addr, 0, 5*time.Second).Error())

	c.run(want, []step{{value: "g"}})
	c.waitConverged(c.nodes[:3], want)

	r := n.raft
	assert.Never(t, func() bool { return r.AppliedIndex() > 0 }, time.Second, 50*time.Millisecond,
		"the replica must not apply anything from a state it does not support")
	assert.Empty(t, n.records())
	assert.Equal(t, Unversioned, n.state.Active())

	require.ErrorContains(t, c.restart(n, 2), "failed to load any existing snapshots",
		"the rejected snapshot is kept, and the replica refuses to start with it")

	require.NoError(t, c.restart(n, 3))
	c.waitConverged([]*testNode{n}, want)
}
