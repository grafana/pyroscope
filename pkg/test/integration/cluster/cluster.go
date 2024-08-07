package cluster

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	pm "github.com/prometheus/client_model/go"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	connectapi "github.com/grafana/pyroscope/pkg/api/connect"
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

func listenAddrFlags(listenAddr string) []string {
	return []string{
		"-compactor.ring.instance-addr=" + listenAddr,
		"-distributor.ring.instance-addr=" + listenAddr,
		"-ingester.lifecycler.addr=" + listenAddr,
		"-memberlist.advertise-addr=" + listenAddr,
		"-overrides-exporter.ring.instance-addr=" + listenAddr,
		"-query-frontend.instance-addr=" + listenAddr,
		"-query-scheduler.ring.instance-addr=" + listenAddr,
		"-store-gateway.sharding-ring.instance-addr=" + listenAddr,
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
	listenAddr := "127.0.0.1"
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
			listenAddrFlags("127.0.0.1")...)
		comp.flags = append(comp.flags,
			[]string{
				"-tracing.enabled=false", // data race
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

	countPerTarget := map[string]int{}

	for _, comp := range c.Components {
		countPerTarget[comp.Target]++

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

						return t.querierReadyCheck(ctx, countPerTarget["ingester"], countPerTarget["store-gateway"])
					}
					if t.Target == "distributor" {
						if err := t.httpReadyCheck(ctx); err != nil {
							return err
						}

						return t.distributorReadyCheck(ctx, countPerTarget["ingester"], countPerTarget["distributor"])
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
		connectapi.DefaultClientOptions()...,
	)
}

func (c *Cluster) PushClient() pushv1connect.PusherServiceClient {
	return pushv1connect.NewPusherServiceClient(
		c.httpClient,
		"http://push",
		connectapi.DefaultClientOptions()...,
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

type gatherCheck struct {
	g          prometheus.Gatherer
	conditions []gatherCoditions
}

//nolint:unparam
func (c *gatherCheck) addExpectValue(value float64, metricName string, labelPairs ...string) *gatherCheck {
	c.conditions = append(c.conditions, gatherCoditions{
		metricName:    metricName,
		labelPairs:    labelPairs,
		expectedValue: value,
	})
	return c
}

type gatherCoditions struct {
	metricName    string
	labelPairs    []string
	expectedValue float64
}

func (c *gatherCoditions) String() string {
	b := strings.Builder{}
	b.WriteString(c.metricName)
	b.WriteRune('{')
	for i := 0; i < len(c.labelPairs); i += 2 {
		b.WriteString(c.labelPairs[i])
		b.WriteRune('=')
		b.WriteString(c.labelPairs[i+1])
		b.WriteRune(',')
	}
	s := b.String()
	return s[:len(s)-1] + "}"
}

func (c *gatherCoditions) matches(pairs []*pm.LabelPair) bool {
outer:
	for i := 0; i < len(c.labelPairs); i += 2 {
		for _, l := range pairs {
			if l.GetName() != c.labelPairs[i] {
				continue
			}
			if l.GetValue() == c.labelPairs[i+1] {
				continue outer // match move to next pair
			}
			return false // value wrong
		}
		return false // label not found
	}
	return true
}

func (comp *Component) checkMetrics() *gatherCheck {
	return &gatherCheck{
		g: comp.reg,
	}
}

func (g *gatherCheck) run(ctx context.Context) error {
	actualValues := make([]float64, len(g.conditions))

	// maps from metric name to condition index
	nameMap := make(map[string][]int)
	for idx, c := range g.conditions {
		// not a number
		actualValues[idx] = math.NaN()
		nameMap[c.metricName] = append(nameMap[c.metricName], idx)
	}

	// now gather actual metrics
	metrics, err := g.g.Gather()
	if err != nil {
		return err
	}

	for _, m := range metrics {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		conditions, ok := nameMap[m.GetName()]
		if !ok {
			continue
		}

		// now iterate over all label pairs
		for _, sm := range m.GetMetric() {
			// check for each condition if it matches with he labels
			for _, condIdx := range conditions {
				if g.conditions[condIdx].matches(sm.Label) {
					actualValues[condIdx] = sm.GetGauge().GetValue() // TODO: handle other types
				}
			}
		}
	}

	errs := make([]error, len(actualValues))
	for idx, actual := range actualValues {
		cond := g.conditions[idx]
		if math.IsNaN(actual) {
			errs[idx] = fmt.Errorf("metric for %s not found", cond.String())
			continue
		}
		if actual != cond.expectedValue {
			errs[idx] = fmt.Errorf("unexpected value for %s: expected %f, got %f", cond.String(), cond.expectedValue, actual)
		}
	}

	return errors.Join(errs...)
}

func (comp *Component) querierReadyCheck(ctx context.Context, expectedIngesters, expectedStoreGateways int) (err error) {
	check := comp.checkMetrics().
		addExpectValue(float64(expectedIngesters), "pyroscope_ring_members", "name", "ingester", "state", "ACTIVE").
		addExpectValue(float64(expectedStoreGateways), "pyroscope_ring_members", "name", "store-gateway-client", "state", "ACTIVE")
	return check.run(ctx)
}

func (comp *Component) distributorReadyCheck(ctx context.Context, expectedIngesters, expectedDistributors int) (err error) {
	check := comp.checkMetrics().
		addExpectValue(float64(expectedIngesters), "pyroscope_ring_members", "name", "ingester", "state", "ACTIVE").
		addExpectValue(float64(expectedDistributors), "pyroscope_ring_members", "name", "distributor", "state", "ACTIVE")
	return check.run(ctx)
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

var lockRegistry sync.Mutex

func (comp *Component) start(_ context.Context) (*phlare.Phlare, error) {
	fs := flag.NewFlagSet(comp.nodeName(), flag.PanicOnError)
	if err := cfg.DynamicUnmarshal(&comp.cfg, comp.flags, fs); err != nil {
		return nil, err
	}

	// Hack to avoid clashing metrics, we should track down the use of globals
	// restore oldReg := prometheus.DefaultRegisterer
	comp.reg = prometheus.NewRegistry()
	lockRegistry.Lock()
	defer lockRegistry.Unlock()
	prometheus.DefaultRegisterer = comp.reg
	prometheus.DefaultGatherer = comp.reg
	comp.cfg.Server.Gatherer = comp.reg
	f, err := phlare.New(comp.cfg)
	if err != nil {
		return nil, err
	}

	return f, nil
}
