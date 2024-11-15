package metastore

import (
	"context"
	"strconv"

	"connectrpc.com/connect"
	"github.com/hashicorp/raft"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type OperatorService struct {
	metastorev1.OperatorServiceServer

	config Config
	raft   *raft.Raft
}

func NewOperatorService(config Config, raft *raft.Raft) *OperatorService {
	return &OperatorService{
		config: config,
		raft:   raft,
	}
}

func (svc *OperatorService) Info(_ context.Context, _ *connect.Request[metastorev1.InfoRequest]) (*connect.Response[metastorev1.InfoResponse], error) {
	cfgFuture := svc.raft.GetConfiguration()
	err := cfgFuture.Error()
	if err != nil {
		return nil, err
	}

	cfg := cfgFuture.Configuration()
	_, leaderID := svc.raft.LeaderWithID()
	stats := svc.raft.Stats()

	res := &metastorev1.InfoResponse{
		Id:       svc.config.Raft.ServerID,
		State:    metastorev1.State(svc.raft.State()),
		LeaderId: string(leaderID),
		Term:     getUint64(stats, "term"),
		Log: &metastorev1.Log{
			CommitIndex:      svc.raft.CommitIndex(),
			AppliedIndex:     svc.raft.AppliedIndex(),
			LastIndex:        svc.raft.LastIndex(),
			FsmPendingLength: getUint64(stats, "fsm_pending"),
		},
		Snapshot: &metastorev1.Snapshot{
			LastIndex: getUint64(stats, "snapshot_last_index"),
			LastTerm:  getUint64(stats, "last_snapshot_term"),
		},
		Protocol: &metastorev1.Protocol{
			Version:            getUint64(stats, "protocol_version"),
			MinVersion:         getUint64(stats, "protocol_max_version"),
			MaxVersion:         getUint64(stats, "protocol_min_version"),
			MinSnapshotVersion: getUint64(stats, "snapshot_version_max"),
			MaxSnapshotVersion: getUint64(stats, "snapshot_version_min"),
		},
	}

	// Perform a more reliable leader check to verify if this node is indeed a
	// leader. A node may report itself as a leader, but not be a leader by
	// consensus of the cluster.
	leaderErr := svc.raft.VerifyLeader().Error()

	switch svc.raft.State() {
	case raft.Leader:
		res.LastLeaderContact = 0
		res.IsStateVerified = leaderErr == nil
	default:
		res.LastLeaderContact = svc.raft.LastContact().UnixMilli()
		res.IsStateVerified = leaderErr == raft.ErrNotLeader
	}

	if len(cfg.Servers) > 1 {
		res.Peers = make([]*metastorev1.Peer, 0, len(cfg.Servers)-1)
		for _, server := range cfg.Servers {
			if string(server.ID) == svc.config.Raft.ServerID || string(server.Address) == svc.config.Raft.ServerID {
				res.Suffrage = metastorev1.Suffrage(server.Suffrage)
				continue
			}

			res.Peers = append(res.Peers, &metastorev1.Peer{
				Id:       string(server.ID),
				Address:  string(server.Address),
				Suffrage: metastorev1.Suffrage(server.Suffrage),
			})
		}
	} else {
		res.Peers = []*metastorev1.Peer{}
	}

	return connect.NewResponse(res), nil
}

// getUint64 tries to get a uint64 value from a map. If the key does not exist
// or the value is not a valid uint64, it returns 0.
func getUint64(m map[string]string, key string) uint64 {
	value, ok := m[key]
	if !ok {
		return 0
	}

	u, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0
	}

	return u
}
