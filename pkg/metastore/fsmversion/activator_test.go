package fsmversion

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/v2/pkg/metastore/fsm"
	"github.com/grafana/pyroscope/v2/pkg/metastore/raftnode/raftnodepb"
)

type fakeRaft struct {
	mu        sync.Mutex
	infos     []*raftnodepb.NodeInfo
	calls     int
	proposals []*raft_log.SetFSMVersionRequest
	propose   func(*raft_log.SetFSMVersionRequest) (proto.Message, error)
}

func (f *fakeRaft) NodeInfo() (*raftnodepb.NodeInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := min(f.calls, len(f.infos)-1)
	f.calls++
	return proto.Clone(f.infos[i]).(*raftnodepb.NodeInfo), nil
}

func (f *fakeRaft) Propose(_ context.Context, t fsm.RaftLogEntryType, m proto.Message) (proto.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t != fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_SET_FSM_VERSION) {
		return nil, errors.New("unexpected command")
	}
	req := m.(*raft_log.SetFSMVersionRequest)
	f.proposals = append(f.proposals, req)
	if f.propose != nil {
		return f.propose(req)
	}
	return &raft_log.SetFSMVersionResponse{Version: req.Version}, nil
}

func (f *fakeRaft) proposed() []*raft_log.SetFSMVersionRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*raft_log.SetFSMVersionRequest(nil), f.proposals...)
}

type fakeNodes struct {
	infos []*raftnodepb.NodeInfo
	err   error
}

func (f *fakeNodes) NodeInfoAll(context.Context) ([]*raftnodepb.NodeInfo, error) {
	return f.infos, f.err
}

func leaderInfo(term, configIndex uint64, peers ...string) *raftnodepb.NodeInfo {
	info := &raftnodepb.NodeInfo{
		ServerId:           peers[0],
		State:              raft.Leader.String(),
		CurrentTerm:        term,
		ConfigurationIndex: configIndex,
	}
	for _, p := range peers {
		info.Peers = append(info.Peers, &raftnodepb.NodeInfo_Peer{ServerId: p, Suffrage: raft.Voter.String()})
	}
	return info
}

func reported(id string, v Version) *raftnodepb.NodeInfo {
	return &raftnodepb.NodeInfo{ServerId: id, SupportedFsmVersion: uint32(v)}
}

func newTestActivator(config Config, supported, active Version, r *fakeRaft, nodes *fakeNodes) *Activator {
	s := NewState(log.NewNopLogger(), supported, nil)
	s.setActive(active)
	return NewActivator(log.NewNopLogger(), config, s, r, nodes)
}

func TestActivator_activate(t *testing.T) {
	peers := []string{"a", "b", "c"}

	tests := []struct {
		name      string
		config    Config
		supported Version
		active    Version
		local     *raftnodepb.NodeInfo
		nodes     *fakeNodes
		want      []*raft_log.SetFSMVersionRequest
		wantErr   bool
	}{
		{
			name:      "activates the version supported by all replicas",
			supported: 3,
			local:     leaderInfo(7, 1, peers...),
			nodes:     &fakeNodes{infos: []*raftnodepb.NodeInfo{reported("b", 3), reported("c", 3)}},
			want:      []*raft_log.SetFSMVersionRequest{{Term: 7, Version: 3}},
		},
		{
			name:      "activates the lowest version supported by all replicas",
			supported: 3,
			active:    1,
			local:     leaderInfo(7, 1, peers...),
			nodes:     &fakeNodes{infos: []*raftnodepb.NodeInfo{reported("b", 2), reported("c", 3)}},
			want:      []*raft_log.SetFSMVersionRequest{{Term: 7, Version: 2}},
		},
		{
			name:      "does nothing when a replica does not support a newer version",
			supported: 3,
			active:    2,
			local:     leaderInfo(7, 1, peers...),
			nodes:     &fakeNodes{infos: []*raftnodepb.NodeInfo{reported("b", 3), reported("c", 2)}},
		},
		{
			name:      "does nothing when a replica does not report a version",
			supported: 3,
			local:     leaderInfo(7, 1, peers...),
			nodes:     &fakeNodes{infos: []*raftnodepb.NodeInfo{reported("b", 3), reported("c", Unversioned)}},
		},
		{
			name:      "fails when a replica is unreachable",
			supported: 3,
			local:     leaderInfo(7, 1, peers...),
			nodes:     &fakeNodes{infos: []*raftnodepb.NodeInfo{reported("b", 3)}, err: errors.New("c: unavailable")},
			wantErr:   true,
		},
		{
			name:      "fails when a replica is missing from discovery",
			supported: 3,
			local:     leaderInfo(7, 1, peers...),
			nodes:     &fakeNodes{infos: []*raftnodepb.NodeInfo{reported("b", 3)}},
			wantErr:   true,
		},
		{
			name:      "uses the lowest version when a replica is reported twice",
			supported: 3,
			active:    1,
			local:     leaderInfo(7, 1, peers...),
			nodes:     &fakeNodes{infos: []*raftnodepb.NodeInfo{reported("b", 3), reported("c", 3), reported("b", 2)}},
			want:      []*raft_log.SetFSMVersionRequest{{Term: 7, Version: 2}},
		},
		{
			name:      "ignores servers that are not raft members",
			supported: 2,
			local:     leaderInfo(7, 1, peers...),
			nodes:     &fakeNodes{infos: []*raftnodepb.NodeInfo{reported("b", 2), reported("c", 2), reported("d", 0)}},
			want:      []*raft_log.SetFSMVersionRequest{{Term: 7, Version: 2}},
		},
		{
			name:      "uses the local supported version for the leader",
			supported: 2,
			local:     leaderInfo(7, 1, peers...),
			nodes:     &fakeNodes{infos: []*raftnodepb.NodeInfo{reported("a", 0), reported("b", 2), reported("c", 2)}},
			want:      []*raft_log.SetFSMVersionRequest{{Term: 7, Version: 2}},
		},
		{
			name:      "activates in a single-node cluster",
			supported: 2,
			local:     leaderInfo(7, 1, "a"),
			nodes:     &fakeNodes{},
			want:      []*raft_log.SetFSMVersionRequest{{Term: 7, Version: 2}},
		},
		{
			name:      "respects the max version",
			config:    Config{MaxVersion: 2},
			supported: 3,
			local:     leaderInfo(7, 1, peers...),
			nodes:     &fakeNodes{infos: []*raftnodepb.NodeInfo{reported("b", 3), reported("c", 3)}},
			want:      []*raft_log.SetFSMVersionRequest{{Term: 7, Version: 2}},
		},
		{
			name:      "does nothing when the max version is already active",
			config:    Config{MaxVersion: 2},
			supported: 3,
			active:    2,
			local:     leaderInfo(7, 1, peers...),
			nodes:     &fakeNodes{infos: []*raftnodepb.NodeInfo{reported("b", 3), reported("c", 3)}},
		},
		{
			name:      "does nothing when the latest version is active",
			supported: 3,
			active:    3,
			local:     leaderInfo(7, 1, peers...),
			nodes:     &fakeNodes{err: errors.New("must not be called")},
		},
		{
			name:      "does nothing on a follower",
			supported: 3,
			local:     &raftnodepb.NodeInfo{ServerId: "a", State: raft.Follower.String()},
			nodes:     &fakeNodes{infos: []*raftnodepb.NodeInfo{reported("b", 3), reported("c", 3)}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &fakeRaft{infos: []*raftnodepb.NodeInfo{tt.local}}
			a := newTestActivator(tt.config, tt.supported, tt.active, r, tt.nodes)
			err := a.activate(context.Background(), new(pendingActivation))
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, r.proposed())
		})
	}
}

func TestActivator_activate_nonVoter(t *testing.T) {
	local := leaderInfo(7, 1, "a", "b", "c")
	local.Peers[2].Suffrage = raft.Nonvoter.String()
	r := &fakeRaft{infos: []*raftnodepb.NodeInfo{local}}

	nodes := &fakeNodes{infos: []*raftnodepb.NodeInfo{reported("b", 2), reported("c", 1)}}
	a := newTestActivator(Config{}, 2, 1, r, nodes)
	require.NoError(t, a.activate(context.Background(), new(pendingActivation)))
	assert.Empty(t, r.proposed(), "non-voters apply the log too")

	nodes.infos = []*raftnodepb.NodeInfo{reported("b", 2), reported("c", 2)}
	require.NoError(t, a.activate(context.Background(), new(pendingActivation)))
	assert.Equal(t, []*raft_log.SetFSMVersionRequest{{Term: 7, Version: 2}}, r.proposed())
}

func TestActivator_activate_membershipChange(t *testing.T) {
	tests := []struct {
		name   string
		before *raftnodepb.NodeInfo
		after  *raftnodepb.NodeInfo
	}{
		{
			name:   "configuration changed",
			before: leaderInfo(7, 1, "a", "b"),
			after:  leaderInfo(7, 2, "a", "b", "c"),
		},
		{
			name:   "term changed",
			before: leaderInfo(7, 1, "a", "b"),
			after:  leaderInfo(8, 1, "a", "b"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &fakeRaft{infos: []*raftnodepb.NodeInfo{tt.before, tt.after}}
			nodes := &fakeNodes{infos: []*raftnodepb.NodeInfo{reported("b", 2), reported("c", 1)}}
			a := newTestActivator(Config{}, 2, 1, r, nodes)
			require.NoError(t, a.activate(context.Background(), new(pendingActivation)))
			assert.Empty(t, r.proposed())
		})
	}
}

func TestActivator_activate_proposal(t *testing.T) {
	t.Run("returns the propose error", func(t *testing.T) {
		r := &fakeRaft{
			infos: []*raftnodepb.NodeInfo{leaderInfo(7, 1, "a")},
			propose: func(*raft_log.SetFSMVersionRequest) (proto.Message, error) {
				return nil, raft.ErrLeadershipLost
			},
		}
		a := newTestActivator(Config{}, 2, 1, r, &fakeNodes{})
		require.ErrorIs(t, a.activate(context.Background(), new(pendingActivation)), raft.ErrLeadershipLost)
	})

	t.Run("tolerates a rejected proposal", func(t *testing.T) {
		r := &fakeRaft{
			infos: []*raftnodepb.NodeInfo{leaderInfo(7, 1, "a")},
			propose: func(*raft_log.SetFSMVersionRequest) (proto.Message, error) {
				return &raft_log.SetFSMVersionResponse{Version: 1}, nil
			},
		}
		a := newTestActivator(Config{}, 2, 1, r, &fakeNodes{})
		require.NoError(t, a.activate(context.Background(), new(pendingActivation)))
		assert.Len(t, r.proposed(), 1)
	})

	t.Run("tolerates an empty response", func(t *testing.T) {
		r := &fakeRaft{
			infos: []*raftnodepb.NodeInfo{leaderInfo(7, 1, "a")},
			propose: func(*raft_log.SetFSMVersionRequest) (proto.Message, error) {
				return nil, nil
			},
		}
		a := newTestActivator(Config{}, 2, 1, r, &fakeNodes{})
		require.NoError(t, a.activate(context.Background(), new(pendingActivation)))
	})
}

func TestActivator_activate_delay(t *testing.T) {
	now := time.Unix(1000, 0)
	nodes := &fakeNodes{infos: []*raftnodepb.NodeInfo{reported("b", 3), reported("c", 3)}}
	r := &fakeRaft{infos: []*raftnodepb.NodeInfo{leaderInfo(7, 1, "a", "b", "c")}}
	a := newTestActivator(Config{ActivationDelay: time.Hour}, 3, 1, r, nodes)
	a.now = func() time.Time { return now }
	pending := new(pendingActivation)

	require.NoError(t, a.activate(context.Background(), pending))
	assert.Empty(t, r.proposed(), "the first observation only schedules the activation")
	assert.Equal(t, pendingActivation{version: 3, since: now}, *pending)

	now = now.Add(30 * time.Minute)
	require.NoError(t, a.activate(context.Background(), pending))
	assert.Empty(t, r.proposed())

	nodes.infos = []*raftnodepb.NodeInfo{reported("b", 3), reported("c", 2)}
	require.NoError(t, a.activate(context.Background(), pending))
	assert.Empty(t, r.proposed())
	assert.Equal(t, pendingActivation{version: 2, since: now}, *pending, "a lower target restarts the delay")

	now = now.Add(59 * time.Minute)
	require.NoError(t, a.activate(context.Background(), pending))
	assert.Empty(t, r.proposed())

	now = now.Add(time.Minute)
	require.NoError(t, a.activate(context.Background(), pending))
	assert.Equal(t, []*raft_log.SetFSMVersionRequest{{Term: 7, Version: 2}}, r.proposed())
	assert.Equal(t, pendingActivation{}, *pending)
}

func TestActivator_activate_delayResetWhenNotNeeded(t *testing.T) {
	now := time.Unix(1000, 0)
	nodes := &fakeNodes{infos: []*raftnodepb.NodeInfo{reported("b", 2)}}
	r := &fakeRaft{infos: []*raftnodepb.NodeInfo{leaderInfo(7, 1, "a", "b")}}
	a := newTestActivator(Config{ActivationDelay: time.Hour}, 2, 1, r, nodes)
	a.now = func() time.Time { return now }
	pending := new(pendingActivation)

	require.NoError(t, a.activate(context.Background(), pending))
	assert.Equal(t, Version(2), pending.version)

	nodes.infos = []*raftnodepb.NodeInfo{reported("b", 1)}
	require.NoError(t, a.activate(context.Background(), pending))
	assert.Equal(t, pendingActivation{}, *pending, "the rolled back replica cancels the pending activation")

	nodes.infos = []*raftnodepb.NodeInfo{reported("b", 2)}
	now = now.Add(2 * time.Hour)
	require.NoError(t, a.activate(context.Background(), pending))
	assert.Empty(t, r.proposed(), "the delay starts over")
}

func TestActivator_StartStop(t *testing.T) {
	r := &fakeRaft{infos: []*raftnodepb.NodeInfo{leaderInfo(7, 1, "a")}}
	a := newTestActivator(Config{CheckInterval: 10 * time.Millisecond}, 2, 1, r, &fakeNodes{})

	a.Start()
	a.Start()
	require.Eventually(t, func() bool { return len(r.proposed()) > 0 }, 5*time.Second, 10*time.Millisecond)
	a.Stop()
	a.Stop()

	time.Sleep(20 * time.Millisecond)
	n := len(r.proposed())
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, n, len(r.proposed()), "no proposals after stop")

	a.Start()
	require.Eventually(t, func() bool { return len(r.proposed()) > n }, 5*time.Second, 10*time.Millisecond)
	a.Stop()
}

func TestActivator_disabled(t *testing.T) {
	r := &fakeRaft{infos: []*raftnodepb.NodeInfo{leaderInfo(7, 1, "a")}}
	a := newTestActivator(Config{}, 2, 1, r, &fakeNodes{})
	a.Start()
	time.Sleep(50 * time.Millisecond)
	a.Stop()
	assert.Empty(t, r.proposed())
}
