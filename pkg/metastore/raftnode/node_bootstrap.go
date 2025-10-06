package raftnode

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/backoff"
	"github.com/grafana/dskit/dns"
	"github.com/hashicorp/raft"

	"github.com/grafana/pyroscope/pkg/metastore/discovery"
	"github.com/grafana/pyroscope/pkg/metastore/raftnode/raftnodepb"
)

func (n *Node) bootstrap() error {
	peers, err := n.bootstrapPeersWithRetries()
	if err != nil {
		return fmt.Errorf("failed to resolve peers: %w", err)
	}
	logger := log.With(n.logger,
		"server_id", n.config.ServerID,
		"advertise_address", n.config.AdvertiseAddress,
		"peers", fmt.Sprint(peers))
	lastPeer := peers[len(peers)-1]
	if raft.ServerAddress(n.config.AdvertiseAddress) != lastPeer.Address {
		level.Info(logger).Log("msg", "not the bootstrap node, skipping")
		return nil
	}
	level.Info(logger).Log("msg", "bootstrapping raft")
	bootstrap := n.raft.BootstrapCluster(raft.Configuration{Servers: peers})
	if bootstrapErr := bootstrap.Error(); bootstrapErr != nil {
		if !errors.Is(bootstrapErr, raft.ErrCantBootstrap) {
			return fmt.Errorf("failed to bootstrap raft: %w", bootstrapErr)
		}
	}
	return nil
}

func (n *Node) bootstrapPeersWithRetries() (peers []raft.Server, err error) {
	prov := dns.NewProvider(n.logger, n.reg, dns.MiekgdnsResolverType)
	attempt := func() bool {
		peers, err = n.bootstrapPeers(prov)
		level.Debug(n.logger).Log("msg", "resolving bootstrap peers", "peers", fmt.Sprint(peers), "err", err)
		if err != nil {
			_ = level.Error(n.logger).Log("msg", "failed to resolve bootstrap peers", "err", err)
			return false
		}
		return true
	}
	backoffConfig := backoff.Config{
		MinBackoff: 1 * time.Second,
		MaxBackoff: 10 * time.Second,
		MaxRetries: 20,
	}
	backOff := backoff.New(context.Background(), backoffConfig)
	for backOff.Ongoing() {
		if !attempt() {
			backOff.Wait()
		} else {
			return peers, nil
		}
	}
	return nil, fmt.Errorf("failed to resolve bootstrap peers after %d retries %w", backOff.NumRetries(), err)
}

const autoJoinTimeout = 10 * time.Second

func (n *Node) tryAutoJoin() error {
	// we can only auto-join if there is a real raft cluster running
	ctx, cancel := context.WithTimeout(context.Background(), autoJoinTimeout)
	defer cancel()

	readIndexResp, err := n.raftNodeClient.ReadIndex(ctx, &raftnodepb.ReadIndexRequest{})
	if err != nil {
		return fmt.Errorf("failed to get current term for auto-join: %w", err)
	}

	logger := log.With(n.logger,
		"server_id", n.config.ServerID,
		"advertise_address", n.config.AdvertiseAddress)

	// try to join the cluster via the leader
	level.Info(logger).Log("msg", "attempting to join existing cluster", "current_term", readIndexResp.Term)
	_, err = n.raftNodeClient.AddNode(ctx, &raftnodepb.AddNodeRequest{
		ServerId:    n.config.AdvertiseAddress,
		CurrentTerm: readIndexResp.Term,
	})

	if err != nil {
		return fmt.Errorf("failed to auto-join cluster: %w", err)
	}

	return nil
}

func (n *Node) bootstrapPeers(prov *dns.Provider) ([]raft.Server, error) {
	// The peer list always includes the local node.
	peers := make([]raft.Server, 0, len(n.config.BootstrapPeers)+1)
	peers = append(peers, raft.Server{
		Suffrage: raft.Voter,
		ID:       raft.ServerID(n.config.ServerID),
		Address:  raft.ServerAddress(n.config.AdvertiseAddress),
	})
	// Note that raft requires stable node IDs, therefore we're using
	// the node FQDN:port for both purposes: as the identifier and as the
	// address. This requires a DNS SRV record lookup without further
	// resolution of A records (dnssrvnoa+).
	//
	// Alternatively, peers may be specified explicitly in the
	// "{addr}</{node_id}>" format, where the node is the optional node
	// identifier.
	var resolve []string
	for _, peer := range n.config.BootstrapPeers {
		if strings.Contains(peer, "+") {
			resolve = append(resolve, peer)
		} else {
			peers = append(peers, discovery.ParsePeer(peer))
		}
	}
	if len(resolve) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := prov.Resolve(ctx, resolve); err != nil {
			return nil, fmt.Errorf("failed to resolve bootstrap peers: %w", err)
		}
		resolvedPeers := prov.Addresses()
		if len(resolvedPeers) == 0 {
			// The local node is the only one in the cluster, but peers
			// are expected to be present. Stop here to avoid bootstrapping
			// a single-node cluster.
			return nil, fmt.Errorf("bootstrap peers can't be resolved")
		}
		for _, peer := range resolvedPeers {
			peers = append(peers, raft.Server{
				Suffrage: raft.Voter,
				ID:       raft.ServerID(peer),
				Address:  raft.ServerAddress(peer),
			})
		}
	}
	// Finally, we sort and deduplicate the peers: the first one
	// is to boostrap the cluster. If there are nodes with distinct
	// IDs but the same address, bootstrapping will fail.
	slices.SortFunc(peers, func(a, b raft.Server) int {
		return strings.Compare(string(a.ID), string(b.ID))
	})
	peers = slices.CompactFunc(peers, func(a, b raft.Server) bool {
		return a.ID == b.ID
	})
	if len(peers) != n.config.BootstrapExpectPeers {
		return nil, fmt.Errorf("expected number of bootstrap peers not reached: got %d, expected %d\n%+v",
			len(peers), n.config.BootstrapExpectPeers, peers)
	}
	return peers, nil
}
