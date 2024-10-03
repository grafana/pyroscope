package discovery

import (
	"context"
	"fmt"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/dns"
	"github.com/hashicorp/raft"
	"sync"
)

type DNSDiscovery struct {
	logger   log.Logger
	addr     string
	provider *dns.Provider
	m        sync.Mutex
	upd      Updates
	resolved []Server
}

func NewDNSDiscovery(l log.Logger, addr string, p *dns.Provider) *DNSDiscovery {
	d := &DNSDiscovery{
		logger:   log.With(l, "addr", addr, "component", "dns-discovery"),
		addr:     addr,
		provider: p,
	}

	return d
}

func (d *DNSDiscovery) Subscribe(updates Updates) {
	d.m.Lock()
	d.upd = updates
	d.m.Unlock()
	d.resolve()
}

func (d *DNSDiscovery) Rediscover() {
	d.resolve()
}

func (d *DNSDiscovery) Close() {

}

func (d *DNSDiscovery) resolve() {
	err := d.provider.Resolve(context.Background(), []string{d.addr})
	if err != nil {
		level.Error(d.logger).Log("msg", "failed to resolve DNS", "addr", d.addr, "err", err)
		return
	}
	addrs := d.provider.Addresses()
	if len(addrs) == 0 {
		level.Error(d.logger).Log("msg", "failed to resolve DNS", "addr", d.addr, "err", "no addresses")
		return
	}
	level.Debug(d.logger).Log("msg", "resolved DNS", "addr", d.addr, "addrs", fmt.Sprintf("%+v", addrs))

	servers := make([]Server, 0, len(addrs))
	for _, peer := range addrs {
		servers = append(servers, Server{
			Raft: raft.Server{
				Suffrage: raft.Voter,
				ID:       raft.ServerID(peer),
				Address:  raft.ServerAddress(peer),
			},
		})
	}
	d.m.Lock()
	defer d.m.Unlock()
	d.resolved = servers
	if d.upd != nil {
		d.upd.Servers(servers)
	}
}
