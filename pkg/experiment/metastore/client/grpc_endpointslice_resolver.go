package metastoreclient

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-kit/log"
	"github.com/prometheus/prometheus/discovery"
	promk8s "github.com/prometheus/prometheus/discovery/kubernetes"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"google.golang.org/grpc/resolver"
)

const GrpcEndpointSLiceResovlerSchema = "metastore-endpointslice"

type EndpointSliceResolverBuilder struct {
	l         log.Logger
	name      string
	namespace string
	port      string
}

func NewGrpcResolverBuilder(l log.Logger, address string) (*EndpointSliceResolverBuilder, error) {
	g := &EndpointSliceResolverBuilder{l: log.With(l, "component", "metastore-grpc-resolver-builder")}
	name, namespace, port, err := getEndpointSliceTargetFromDnsTarget(address)
	if err != nil {
		return nil, fmt.Errorf("failed to parse target: %w", err)
	}
	g.name = name
	g.namespace = namespace
	g.port = port
	g.l.Log("msg", "created new grpc resolver builder", "name", name, "namespace", namespace, "port", port)

	return g, nil
}

func (g *EndpointSliceResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	g.l.Log("msg", "building resolver", "target", target, "opts", fmt.Sprintf("%+v", opts))
	rr := &GrpcResolver{w: nil, l: log.With(g.l, "component", "metastore-grpc-resolver", "target", target)}
	newWatcher, err := NewEndpointSliceWatcher(g.l, g.name, g.namespace, func(ips []string) {
		addresses := make([]resolver.Address, 0, len(ips))
		for _, ip := range ips {
			addresses = append(addresses, resolver.Address{Addr: ip + ":" + g.port})
		}
		err := cc.UpdateState(resolver.State{Addresses: addresses})
		if err != nil {
			rr.l.Log("msg", "failed to update state", "err", err)
		} else {
			rr.l.Log("msg", "updated state", "addresses", fmt.Sprintf("%+v", addresses))
		}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}
	rr.w = newWatcher
	return rr, nil
}

func (g *EndpointSliceResolverBuilder) Scheme() string {
	return GrpcEndpointSLiceResovlerSchema
}

func (g *EndpointSliceResolverBuilder) resolverAddrStub() string {
	return fmt.Sprintf("%s://stub:239", GrpcEndpointSLiceResovlerSchema)
}

type GrpcResolver struct {
	w *EndpointSliceWatcher
	l log.Logger
}

func (g *GrpcResolver) ResolveNow(o resolver.ResolveNowOptions) {
	//g.l.Log("msg", "resolve now", "opts", o)
}

func (g *GrpcResolver) Close() {
	g.l.Log("msg", "close")
	g.w.Close()
}

func getEndpointSliceTargetFromDnsTarget(src string) (string, string, string, error) {

	re := regexp.MustCompile("dns:///_grpc._tcp\\.([\\S^.]+)\\.([\\S^.]+)(\\.svc\\.cluster\\.local\\.):([0-9]+)")
	all := re.FindSubmatch([]byte(src))
	if len(all) == 0 {
		return "", "", "", fmt.Errorf("failed to parse target")
	}
	name := string(all[1])
	namespace := string(all[2])
	port := string(all[4])
	return name, namespace, port, nil
}

type EndpointSliceWatcher struct {
	l         log.Logger
	d         discovery.Discoverer
	ctx       context.Context
	cancel    context.CancelFunc
	name      string
	namespace string
	cb        func(ips []string)
}

func (w *EndpointSliceWatcher) watch(up chan []*targetgroup.Group) {
	isNeededSlice := func(group *targetgroup.Group) bool { //todo proper selection
		//if strings.Contains(group.Source, "endpointslice/"+"pyroscope"+"/"+"pyroscope-micro-services-metastore-headless") {
		//	return true
		//}
		//if strings.Contains(group.Source, "endpointslice/"+"profiles-dev-003"+"/"+"pyroscope-metastore-headless") {
		//	return true
		//}
		substr := "endpointslice/" + w.namespace + "/" + w.name
		w.l.Log("msg", "checking group", "source", group.Source, "substr", substr)
		if strings.Contains(group.Source, substr) {
			return true
		}
		return false
	}
	w.l.Log("msg", "starting watch")
	for {
		select {
		case <-w.ctx.Done():
			w.l.Log("msg", "context done, stopping watch")
			return
		case groups := <-up:
			ipset := make(map[string]string)
			for _, group := range groups {
				if !isNeededSlice(group) {
					w.l.Log("msg", "skipping group", "source", group.Source)
					continue
				}
				w.l.Log("msg", "processing group", "source", group.Source)
				for _, target := range group.Targets {
					ip := target["__meta_kubernetes_pod_ip"]
					ready := target["__meta_kubernetes_pod_ready"]
					phase := target["__meta_kubernetes_pod_phase"]
					podname := target["__meta_kubernetes_pod_name"]
					w.l.Log("msg", "received new target", "tt", fmt.Sprintf(">>%s %s %s<<", ip, phase, ready))
					ipset[string(ip)] = string(podname)
				}
			}
			if len(ipset) == 0 {
				continue
			}
			if w.cb != nil {
				ipss := make([]string, 0, len(ipset))
				for k := range ipset {
					ipss = append(ipss, k)
				}
				w.cb(ipss)
			}
			w.l.Log("msg", "received new target groups", "ips", fmt.Sprintf("%+v", ipset))
		}
	}
}

func (w *EndpointSliceWatcher) Close() error {
	w.cancel()
	return nil
}

func NewEndpointSliceWatcher(l log.Logger, name, namespace string, cb func(ips []string)) (*EndpointSliceWatcher, error) {
	l = log.With(l, "component", "metastore-watcher")
	sdc := &promk8s.SDConfig{
		Role: promk8s.RoleEndpointSlice,
		Selectors: []promk8s.SelectorConfig{
			{
				Role:  promk8s.RoleEndpointSlice,
				Label: "app.kubernetes.io/component=metastore",
			},
		},
	}
	refreshMetrics := discovery.NewRefreshMetrics(nil)
	m := sdc.NewDiscovererMetrics(nil, refreshMetrics)
	d, err := sdc.NewDiscoverer(discovery.DiscovererOptions{
		Logger:            log.With(l, "component", "metastore-watcher-discovery"),
		Metrics:           m,
		HTTPClientOptions: nil,
	})
	if err != nil {
		l.Log("msg", "failed to create discoverer", "err", err)
		return nil, err
	}
	ctx, cacnel := context.WithCancel(context.Background())
	up := make(chan []*targetgroup.Group)

	l.Log("msg", "starting watcher")
	w := &EndpointSliceWatcher{
		d:         d,
		cancel:    cacnel,
		ctx:       ctx,
		l:         l,
		cb:        cb,
		name:      name,
		namespace: namespace,
	}
	go d.Run(ctx, up)
	go w.watch(up)
	return w, nil
}
