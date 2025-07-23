package raftnode

import (
	"sort"

	"github.com/grafana/pyroscope/pkg/metastore/raftnode/raftnodepb"
	"github.com/grafana/pyroscope/pkg/util/build"
)

func (n *Node) NodeInfo() (*raftnodepb.NodeInfo, error) {
	configFuture := n.raft.GetConfiguration()
	err := configFuture.Error()
	if err != nil {
		return nil, err
	}

	config := configFuture.Configuration()
	_, leaderID := n.raft.LeaderWithID()

	info := &raftnodepb.NodeInfo{
		ServerId:           n.config.ServerID,
		AdvertisedAddress:  n.config.AdvertiseAddress,
		State:              n.raft.State().String(),
		LeaderId:           string(leaderID),
		CommitIndex:        n.raft.CommitIndex(),
		AppliedIndex:       n.raft.AppliedIndex(),
		LastIndex:          n.raft.LastIndex(),
		Stats:              statsProto(n.raft.Stats()),
		Peers:              make([]*raftnodepb.NodeInfo_Peer, len(config.Servers)),
		ConfigurationIndex: configFuture.Index(),
		CurrentTerm:        n.raft.CurrentTerm(),
		BuildVersion:       build.Version,
		BuildRevision:      build.Revision,
	}

	for i, server := range config.Servers {
		info.Peers[i] = &raftnodepb.NodeInfo_Peer{
			ServerId:      string(server.ID),
			ServerAddress: string(server.Address),
			Suffrage:      server.Suffrage.String(),
		}
	}

	return info, nil
}

func statsProto(m map[string]string) *raftnodepb.NodeInfo_Stats {
	stats := &raftnodepb.NodeInfo_Stats{
		Name:  make([]string, len(m)),
		Value: make([]string, len(m)),
	}
	var i int
	for name := range m {
		stats.Name[i] = name
		i++
	}
	sort.Strings(stats.Name)
	for j, name := range stats.Name {
		stats.Value[j] = m[name]
	}
	return stats
}
