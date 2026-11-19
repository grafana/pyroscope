package querybackendclient

import (
	"time"

	"google.golang.org/grpc/resolver"
)

// grpc-go only re-resolves successful DNS lookups when prompted, and limits lookups to one every 30 seconds.
const dnsRefreshInterval = 30 * time.Second

type refreshingDNSBuilder struct {
	delegate resolver.Builder
}

func (b refreshingDNSBuilder) Scheme() string { return b.delegate.Scheme() }

func (b refreshingDNSBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	delegate, err := b.delegate.Build(target, cc, opts)
	if err != nil {
		return nil, err
	}
	r := &refreshingDNSResolver{
		delegate: delegate,
		stop:     make(chan struct{}),
		stopped:  make(chan struct{}),
	}
	go r.refresh()
	return r, nil
}

type refreshingDNSResolver struct {
	delegate resolver.Resolver
	stop     chan struct{}
	stopped  chan struct{}
}

func (r *refreshingDNSResolver) refresh() {
	defer close(r.stopped)
	ticker := time.NewTicker(dnsRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.delegate.ResolveNow(resolver.ResolveNowOptions{})
		case <-r.stop:
			return
		}
	}
}

func (r *refreshingDNSResolver) ResolveNow(opts resolver.ResolveNowOptions) {
	r.delegate.ResolveNow(opts)
}

func (r *refreshingDNSResolver) Close() {
	close(r.stop)
	<-r.stopped
	r.delegate.Close()
}
