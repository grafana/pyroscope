package discovery

import (
	"github.com/go-kit/log"
	"github.com/grafana/dskit/dns"
	kuberesolver2 "github.com/grafana/pyroscope/pkg/experiment/metastore/discovery/kuberesolver"
	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus"
	"net"
	"strings"
)

func NewDiscovery(l log.Logger, address string, reg prometheus.Registerer) (Discovery, error) {
	if strings.HasPrefix(address, "dnssrvnoa+") {
		p := dns.NewProvider(l,
			prometheus.WrapRegistererWithPrefix(
				"pyroscope_metastore_client_",
				reg,
			),
			dns.MiekgdnsResolverType)
		return NewDNSDiscovery(l, address, p), nil
	}
	if strings.HasPrefix(address, "kubernetes:///") {
		kubeClient, err := kuberesolver2.NewInClusterK8sClient()
		if err != nil {
			return nil, err
		}
		return NewKubeResolverDiscovery(l, address, kubeClient)
	}
	peers := ParsePeers(address)
	srvs := make([]Server, 0, len(peers))
	for _, peer := range peers {
		srvs = append(srvs, Server{
			Raft:            peer,
			ResolvedAddress: string(peer.Address),
		})
	}
	return NewStaticDiscovery(srvs), nil
}

func ParsePeers(raw string) []raft.Server {
	rpeers := strings.Split(raw, ",")
	peers := make([]raft.Server, 0, len(rpeers))
	for _, rpeer := range rpeers {
		peers = append(peers, ParsePeer(rpeer))
	}
	return peers
}

func ParsePeer(raw string) raft.Server {
	// The string may be "{addr}" or "{addr}/{node_id}".
	parts := strings.SplitN(raw, "/", 2)
	var addr string
	var node string
	if len(parts) == 2 {
		addr = parts[0]
		node = parts[1]
	} else {
		addr = raw
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// No port specified.
		host = addr
	}
	if node == "" {
		// No node_id specified.
		node = host
	}
	return raft.Server{
		Suffrage: raft.Voter,
		ID:       raft.ServerID(node),
		Address:  raft.ServerAddress(addr),
	}
}
