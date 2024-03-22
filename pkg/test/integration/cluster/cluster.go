package cluster

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	"github.com/grafana/pyroscope/pkg/cfg"
	"github.com/grafana/pyroscope/pkg/phlare"
)

func getFreeTCPPorts(address string, count int) ([]int, error) {
	ports := make([]int, count)
	for i := 0; i < count; i++ {
		addr, err := net.ResolveTCPAddr("tcp", address+":0")
		if err != nil {
			return nil, err
		}

		l, err := net.ListenTCP("tcp", addr)
		if err != nil {
			return nil, err
		}
		defer l.Close()

		if tcpAddr, ok := l.Addr().(*net.TCPAddr); ok {
			ports[i] = tcpAddr.Port
		} else {
			return nil, fmt.Errorf("unable to retrieve tcp port from %v", l)
		}
	}

	return ports, nil
}

func newComponent(target string) *Component {
	return &Component{
		Target: target,
	}
}

func NewMicroServiceCluster() *Cluster {
	// use custom http client to resolve dynamically to healthy components

	c := &Cluster{}

	defaultTransport := http.DefaultTransport.(*http.Transport)
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			var err error
			switch addr {
			case "push:80":
				addr, err = c.pickHealthyComponent("distributor")
				if err != nil {
					return nil, err
				}
			case "querier:80":
				addr, err = c.pickHealthyComponent("query-frontend", "querier")
				if err != nil {
					return nil, err
				}
			default:
				return nil, fmt.Errorf("unknown addr %s", addr)
			}

			return defaultTransport.DialContext(ctx, network, addr)
		},
	}
	c.httpClient = &http.Client{Transport: transport}
	c.Components = []*Component{
		newComponent("distributor"),
		newComponent("distributor"),
		newComponent("querier"),
		newComponent("querier"),
		newComponent("ingester"),
		newComponent("ingester"),
		newComponent("ingester"),
		newComponent("store-gateway"),
		newComponent("store-gateway"),
		newComponent("store-gateway"),
	}
	return c
}

type Cluster struct {
	Components []*Component
	wg         sync.WaitGroup // components wait group

	tmpDir     string
	httpClient *http.Client
}

func nodeNameFlags(nodeName string) []string {
	return []string{
		"-memberlist.nodename=" + nodeName,
		"-ingester.lifecycler.ID=" + nodeName,
		"-compactor.ring.instance-id=" + nodeName,
		"-distributor.ring.instance-id=" + nodeName,
		"-overrides-exporter.ring.instance-id=" + nodeName,
		"-query-scheduler.ring.instance-id=" + nodeName,
		"-store-gateway.sharding-ring.instance-id=" + nodeName,
	}
}

func (c *Cluster) pickHealthyComponent(targets ...string) (addr string, err error) {
	results := make([][]string, len(targets))

	for _, comp := range c.Components {
		for i, target := range targets {
			if comp.Target == target {
				results[i] = append(results[i], fmt.Sprintf("%s:%d", "127.0.0.1", comp.ports[0]))
			}
		}
	}

	for _, result := range results {
		if len(result) > 0 {
			// pick random element of list
			return result[rand.Intn(len(result))], nil
		}
	}

	return "", fmt.Errorf("no healthy component found for targets %v", targets)
}

func (c *Cluster) Prepare() (err error) {
	// tmp dir
	c.tmpDir, err = os.MkdirTemp("", "pyroscope-test")
	if err != nil {
		return err
	}
	dataSharedDir := filepath.Join(c.tmpDir, "data-shared")
	if err := os.Mkdir(dataSharedDir, 0o755); err != nil {
		return err
	}

	// allocate two tcp ports per component
	portsPerComponent := 3
	listenAddr := "0.0.0.0"
	ports, err := getFreeTCPPorts(listenAddr, len(c.Components)*portsPerComponent)
	if err != nil {
		return err
	}

	perTarget := map[string]int{}
	for _, c := range c.Components {
		v, ok := perTarget[c.Target]
		if ok {
			v += 1
		}
		perTarget[c.Target] = v
		c.replica = v
	}

	memberlistJoin := []string{}

	for _, comp := range c.Components {
		comp.ports = ports[0:portsPerComponent]
		ports = ports[3:]
		prefix := filepath.Join(c.tmpDir, comp.nodeName())
		dataDir := filepath.Join(prefix, "data")
		compactorDir := filepath.Join(prefix, "data-compactor")
		syncDir := filepath.Join(prefix, "pyroscope-sync")

		for _, dir := range []string{prefix, dataDir, compactorDir, syncDir} {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}

		comp.flags = append(
			nodeNameFlags(comp.nodeName()),
			[]string{
				"-distributor.replication-factor=3",
				"-store-gateway.sharding-ring.replication-factor=3",
				fmt.Sprintf("-target=%s", comp.Target),
				fmt.Sprintf("-memberlist.advertise-port=%d", comp.ports[2]),
				fmt.Sprintf("-memberlist.bind-port=%d", comp.ports[2]),
				fmt.Sprintf("-memberlist.bind-addr=%s", listenAddr),
				fmt.Sprintf("-server.http-listen-port=%d", comp.ports[0]),
				fmt.Sprintf("-server.http-listen-address=%s", listenAddr),
				fmt.Sprintf("-server.grpc-listen-port=%d", comp.ports[1]),
				fmt.Sprintf("-server.grpc-listen-address=%s", listenAddr),
				fmt.Sprintf("-blocks-storage.bucket-store.sync-dir=%s", syncDir),
				fmt.Sprintf("-compactor.data-dir=%s", compactorDir),
				fmt.Sprintf("-pyroscopedb.data-path=%s", dataDir),
				"-storage.backend=filesystem",
				fmt.Sprintf("-storage.filesystem.dir=%s", dataSharedDir),
			}...)

		// handle memberlist join
		for _, m := range memberlistJoin {
			comp.flags = append(comp.flags, fmt.Sprintf("-memberlist.join=%s", m))
		}
		memberlistJoin = append(memberlistJoin, fmt.Sprintf("127.0.0.1:%d", comp.ports[2]))

	}

	return nil
}

func (c *Cluster) Stop() func(context.Context) error {
	funcWaiters := make([]func(context.Context) error, 0, len(c.Components))
	for _, comp := range c.Components {
		funcWaiters = append(funcWaiters, comp.Stop())
	}

	return func(ctx context.Context) error {
		g, ctx := errgroup.WithContext(ctx)
		for _, f := range funcWaiters {
			f := f
			g.Go(func() error {
				return f(ctx)
			})
		}
		return g.Wait()
	}

}

func (c *Cluster) Start(ctx context.Context) (err error) {

	notReady := make(map[*Component]error)

	for _, comp := range c.Components {
		p, err := comp.start(ctx)
		if err != nil {
			return err
		}
		comp.p = p

		notReady[comp] = nil

		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			err := p.Run()
			if err != nil {
				log.Println(err)
			}
		}()

	}

	readyCh := make(chan struct{})
	go func() {
		rate := 200 * time.Millisecond
		ticker := time.NewTicker(rate)
		defer ticker.Stop()
		for {
			for t := range notReady {
				if err := func() error {
					ctx, cancel := context.WithTimeout(context.Background(), rate)
					defer cancel()

					if t.Target == "querier" {
						if err := t.httpReadyCheck(ctx); err != nil {
							return err
						}

						return t.querierReadyCheck(ctx, 3, 3)
					}

					return t.httpReadyCheck(ctx)
				}(); err != nil {
					notReady[t] = err
				} else {
					delete(notReady, t)
				}

			}

			if len(notReady) == 0 {
				close(readyCh)
				break
			}

			<-ticker.C
		}
	}()

	<-readyCh

	return nil
}

func (c *Cluster) Wait() {
	c.wg.Wait()
}

func (c *Cluster) QueryClient() querierv1connect.QuerierServiceClient {
	return querierv1connect.NewQuerierServiceClient(
		c.httpClient,
		"http://querier",
	)
}

func (c *Cluster) PushClient() pushv1connect.PusherServiceClient {
	return pushv1connect.NewPusherServiceClient(
		c.httpClient,
		"http://push",
	)
}

type Component struct {
	Target  string
	replica int
	ports   []int
	flags   []string
	cfg     phlare.Config
	p       *phlare.Phlare
	reg     *prometheus.Registry
}

func (comp *Component) querierReadyCheck(ctx context.Context, expectedIngesters, expectedStoreGateways int) error {
	metrics, err := comp.reg.Gather()
	if err != nil {
		return err
	}

	activeIngesters := 0
	activeStoreGateways := 0

	for _, m := range metrics {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if m.GetName() == "pyroscope_ring_members" {
			for _, sm := range m.GetMetric() {
				foundIngester := false
				foundStoreGateway := false
				foundActive := false
				for _, l := range sm.GetLabel() {
					if l.GetName() == "name" && l.GetValue() == "ingester" {
						foundIngester = true
					}
					if l.GetName() == "name" && l.GetValue() == "store-gateway-client" {
						foundStoreGateway = true
					}
					if l.GetName() == "state" && l.GetValue() == "ACTIVE" {
						foundActive = true
					}
				}
				if foundIngester && foundActive {
					if v := sm.GetGauge().GetValue(); v > 0 {
						activeIngesters = int(v)
					}
				}
				if foundStoreGateway && foundActive {
					if v := sm.GetGauge().GetValue(); v > 0 {
						activeStoreGateways = int(v)
					}
				}
			}
		}
	}

	if activeIngesters != expectedIngesters {
		return fmt.Errorf("expected %d active ingesters, got %d", expectedIngesters, activeIngesters)
	}
	if activeStoreGateways != expectedStoreGateways {
		return fmt.Errorf("expected %d active store gateways, got %d", expectedStoreGateways, activeStoreGateways)
	}

	return nil

}

func (comp *Component) httpReadyCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://127.0.0.1:%d/ready", comp.ports[0]), nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode/100 == 2 {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return fmt.Errorf("status=%d msg=%s", resp.StatusCode, string(body))
}

func (comp *Component) Stop() func(context.Context) error {
	return comp.p.Stop()
}

func (comp *Component) String() string {
	if len(comp.ports) == 3 {
		return fmt.Sprintf("[%s] http=%d grpc=%d memberlist=%d", comp.nodeName(), comp.ports[0], comp.ports[1], comp.ports[2])
	}
	return fmt.Sprintf("[%s]", comp.nodeName())
}

func (comp *Component) nodeName() string {
	return fmt.Sprintf("%s-%d", comp.Target, comp.replica)
}

func (comp *Component) start(_ context.Context) (*phlare.Phlare, error) {
	fs := flag.NewFlagSet(comp.nodeName(), flag.PanicOnError)
	if err := cfg.DynamicUnmarshal(&comp.cfg, comp.flags, fs); err != nil {
		return nil, err
	}

	// Hack to avoid clashing metrics, we should track down the use of globals
	// restore oldReg := prometheus.DefaultRegisterer
	comp.reg = prometheus.NewRegistry()
	prometheus.DefaultRegisterer = comp.reg
	prometheus.DefaultGatherer = comp.reg
	f, err := phlare.New(comp.cfg)
	if err != nil {
		return nil, err
	}

	return f, nil
}
