package metastore

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
)

func (m *Metastore) bootstrap() error {
	peers, err := m.bootstrapPeers()
	if err != nil {
		return fmt.Errorf("failed to resolve peers: %w", err)
	}
	if len(peers) == 0 {
		return fmt.Errorf("no peers found")
	}
	if raft.ServerID(m.config.Raft.ServerID) != peers[0].ID {
		_ = level.Info(m.logger).Log("msg", "skipping raft bootstrap",
			"local", m.config.Raft.ServerID,
			"peers", fmt.Sprint(peers))
		return nil
	}
	bootstrap := m.raft.BootstrapCluster(raft.Configuration{Servers: peers})
	if bootstrapErr := bootstrap.Error(); bootstrapErr != nil {
		if !errors.Is(bootstrapErr, raft.ErrCantBootstrap) {
			return fmt.Errorf("failed to bootstrap raft: %w", bootstrapErr)
		}
	}
	return nil
}

func (m *Metastore) bootstrapPeers() ([]raft.Server, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var peers []raft.Server
	for _, addr := range m.config.Raft.BootstrapPeers {
		parsed, err := peerFromAddress(ctx, addr)
		if err != nil {
			return nil, err
		}
		peers = append(peers, parsed...)
	}
	slices.SortFunc(peers, func(a, b raft.Server) int {
		return strings.Compare(string(a.ID), string(b.ID))
	})
	return peers, nil
}

func peerFromAddress(ctx context.Context, addr string) ([]raft.Server, error) {
	if name, found := strings.CutPrefix(addr, "dns+"); found {
		return lookupPeers(ctx, name)
	}
	p, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	if p.Scheme == "dns" {
		// dns://[authority]/{host[:port]}
		// DNS scheme uses the URL host for authority, and the actual target
		// is in the path token, and the path (name) may have a leading slash.
		return lookupPeers(ctx, strings.TrimPrefix(p.Path, "/"))
	}
	return parsePeer(addr)
}

func parsePeer(raw ...string) ([]raft.Server, error) {
	peers := make([]raft.Server, 0, len(raw))
	for _, str := range raw {
		// The string may be "{addr}" or "{addr}/{node}".
		parts := strings.SplitN(str, "/", 2)
		var addr string
		var node string
		if len(parts) == 2 {
			addr = parts[0]
			node = parts[1]
		} else {
			addr = str
		}
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		if node == "" {
			node = host
		}
		peers = append(peers, raft.Server{
			Suffrage: raft.Voter,
			ID:       raft.ServerID(node),
			Address:  raft.ServerAddress(addr),
		})
	}
	return peers, nil
}

func lookupPeers(ctx context.Context, addr string) ([]raft.Server, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		host, port = addr, ""
	}
	_, recs, err := net.DefaultResolver.LookupSRV(ctx, "", "", host)
	if len(recs) == 0 && err != nil {
		return nil, err
	}
	var peers []raft.Server
	for _, r := range recs {
		// The SRV record may have a port, but we prefer the one from the URL.
		rPort := port
		if rPort == "" {
			rPort = strconv.Itoa(int(r.Port))
		}
		peers = append(peers, raft.Server{
			Suffrage: raft.Voter,
			ID:       raft.ServerID(r.Target),
			Address:  raft.ServerAddress(net.JoinHostPort(r.Target, rPort)),
		})
	}
	return peers, nil
}
