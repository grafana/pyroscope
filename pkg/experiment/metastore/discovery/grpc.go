package discovery

import (
	"fmt"
	"github.com/go-kit/log"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/serviceconfig"
	"net/url"
)

type GrpcDiscovery struct {
	l        log.Logger
	resolver resolver.Resolver
}

func NewGrpcDiscovery(l log.Logger, target string) (*GrpcDiscovery, error) {
	l = log.With(l, "target", target, "component", "metastore-grpc-discovery")
	u, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	gtarget := resolver.Target{
		URL: *u,
	}
	res := &GrpcDiscovery{
		l: l,
	}

	r := resolver.Get(gtarget.URL.Scheme)
	if r == nil {
		return nil, fmt.Errorf("resolver scheme %q not registered (%s)", gtarget.URL.Scheme, target)
	}
	resolver, err := r.Build(gtarget, res, resolver.BuildOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to build resolver: %w", err)
	}
	res.resolver = resolver

	return res, nil
}

func (g *GrpcDiscovery) UpdateState(state resolver.State) error {
	g.NewAddress(state.Addresses)
	return nil
}

func (g *GrpcDiscovery) ReportError(err error) {
	g.resolver.ResolveNow(resolver.ResolveNowOptions{})
}

func (g *GrpcDiscovery) NewAddress(addresses []resolver.Address) {
	g.l.Log("msg", "new address", "addresses", fmt.Sprintf("%+v", addresses))
	for _, address := range addresses {
		g.l.Log("msg", "new address", "address", address.String())
	}
}

func (g *GrpcDiscovery) ParseServiceConfig(serviceConfigJSON string) *serviceconfig.ParseResult {
	return &serviceconfig.ParseResult{
		Config: nil,
		Err:    fmt.Errorf("no implementation for ParseServiceConfig"),
	}
}

//func parseTarget(target string) (resolver.Target, error) {
//  u, err := url.Parse(target)
//  if err != nil {
//    return nil, err
//  }
//
//  return resolver.Target{
//    URL: *u,
//  }
//}
