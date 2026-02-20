package cluster

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	connectapi "github.com/grafana/pyroscope/pkg/api/connect"
	"github.com/grafana/pyroscope/pkg/tenant"
)

const listenAddr = "127.0.0.1"

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

type testTransport struct {
	defaultDialContext func(ctx context.Context, network, addr string) (net.Conn, error)
	next               http.RoundTripper
	c                  *Cluster
}

// use custom http transport to resolve dynamically to healthy components
func newTestTransport(c *Cluster) http.RoundTripper {
	defaultTransport := http.DefaultTransport.(*http.Transport)
	t := &testTransport{
		defaultDialContext: defaultTransport.DialContext,
		c:                  c,
	}
	t.next = &http.Transport{
		Proxy:                 defaultTransport.Proxy,
		TLSClientConfig:       defaultTransport.TLSClientConfig,
		TLSHandshakeTimeout:   defaultTransport.TLSHandshakeTimeout,
		ExpectContinueTimeout: defaultTransport.ExpectContinueTimeout,
		MaxIdleConns:          defaultTransport.MaxIdleConns,
		IdleConnTimeout:       defaultTransport.IdleConnTimeout,
		ForceAttemptHTTP2:     defaultTransport.ForceAttemptHTTP2,
		DialContext:           t.DialContext,
	}
	return t
}

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tenantID, err := tenant.ExtractTenantIDFromContext(req.Context())
	if err == nil {
		req.Header.Set("X-Scope-OrgID", tenantID)
	}
	return t.next.RoundTrip(req)
}

func (t *testTransport) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	var err error
	switch addr {
	case "push:80":
		addr, err = t.c.pickHealthyComponent("distributor")
		if err != nil {
			return nil, err
		}
	case "querier:80":
		addr, err = t.c.pickHealthyComponent("query-frontend", "querier")
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown addr %s", addr)
	}

	return t.defaultDialContext(ctx, network, addr)
}

type ClusterOption func(c *Cluster)

func NewMicroServiceCluster(opts ...ClusterOption) *Cluster {
	c := &Cluster{}
	WithV1()(c)

	// apply options
	for _, opt := range opts {
		opt(c)
	}

	c.httpClient = &http.Client{Transport: newTestTransport(c)}
	c.Components = make([]*Component, len(c.expectedComponents))
	for idx := range c.expectedComponents {
		c.Components[idx] = newComponent(c.expectedComponents[idx])
	}

	return c
}

type Cluster struct {
	Components []*Component
	perTarget  map[string][]int // indexes replicas per target into Components slice

	wg sync.WaitGroup // components wait group

	v2                 bool     // is this a v2 cluster
	debuginfodURL      string   // debuginfod URL for symbolization
	expectedComponents []string // number of expected components

	tmpDir     string
	httpClient *http.Client
}

func (c *Cluster) commonFlags(comp *Component) []string {
	nodeName := comp.nodeName()
	return []string{
		"-auth.multitenancy-enabled=true",
		"-tracing.enabled=false", // data race
		"-self-profiling.disable-push=true",
		fmt.Sprintf("-pyroscopedb.data-path=%s", c.dataDir(comp)),
		"-storage.backend=filesystem",
		fmt.Sprintf("-storage.filesystem.dir=%s", c.dataSharedDir()),
		fmt.Sprintf("-target=%s", comp.Target),
		fmt.Sprintf("-memberlist.advertise-port=%d", comp.memberlistPort),
		fmt.Sprintf("-memberlist.bind-port=%d", comp.memberlistPort),
		fmt.Sprintf("-memberlist.bind-addr=%s", listenAddr),
		"-memberlist.leave-timeout=1s",
		"-memberlist.advertise-addr=" + listenAddr,
		"-memberlist.nodename=" + nodeName,
		fmt.Sprintf("-server.http-listen-port=%d", comp.httpPort),
		fmt.Sprintf("-server.http-listen-address=%s", listenAddr),
		fmt.Sprintf("-server.grpc-listen-port=%d", comp.grpcPort),
		fmt.Sprintf("-server.grpc-listen-address=%s", listenAddr),
		"-distributor.ring.instance-addr=" + listenAddr,
		"-distributor.ring.instance-id=" + nodeName,
		"-distributor.ring.heartbeat-period=1s",
		"-overrides-exporter.ring.instance-addr=" + listenAddr,
		"-overrides-exporter.ring.instance-id=" + nodeName,
		"-overrides-exporter.ring.heartbeat-period=1s",
		"-query-frontend.instance-addr=" + listenAddr,
	}
}

func (c *Cluster) pickHealthyComponent(targets ...string) (addr string, err error) {
	results := make([][]string, len(targets))

	for _, comp := range c.Components {
		for i, target := range targets {
			if comp.Target == target {
				results[i] = append(results[i], fmt.Sprintf("%s:%d", listenAddr, comp.httpPort))
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
func (c *Cluster) dataSharedDir() string {
	return filepath.Join(c.tmpDir, "data-shared")
}

func (c *Cluster) dataDir(comp *Component) string {
	return filepath.Join(c.tmpDir, comp.nodeName(), "data")
}

func (c *Cluster) Prepare(ctx context.Context) (err error) {
	// tmp dir
	c.tmpDir, err = os.MkdirTemp("", "pyroscope-test")
	if err != nil {
		return err
	}
	if err := os.Mkdir(c.dataSharedDir(), 0o755); err != nil {
		return err
	}

	// allocate two tcp ports per component
	portsPerComponent := 3
	if c.v2 {
		portsPerComponent = 4
	}
	ports, err := getFreeTCPPorts(listenAddr, len(c.Components)*portsPerComponent)
	if err != nil {
		return err
	}

	// flags with all components that participate in memberlist
	memberlistJoin := []string{}
	c.perTarget = map[string][]int{}
	for compidx, comp := range c.Components {
		c.perTarget[comp.Target] = append(c.perTarget[comp.Target], compidx)
		comp.replica = len(c.perTarget[comp.Target]) - 1

		// allocate ports
		comp.addPorts(ports[0:portsPerComponent])
		ports = ports[portsPerComponent:]

		// add to memberlist join list
		memberlistJoin = append(memberlistJoin, fmt.Sprintf("%s:%d", listenAddr, comp.memberlistPort))

		if err := os.MkdirAll(c.dataDir(comp), 0o755); err != nil {
			return err
		}
	}

	if c.v2 {
		return c.v2Prepare(ctx, memberlistJoin)
	}

	return c.v1Prepare(ctx, memberlistJoin)
}

func (c *Cluster) Stop() func(context.Context) error {
	funcWaiters := make([]func(context.Context) error, 0, len(c.Components)+1)
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

					var found bool
					var err error

					if c.v2 {
						found, err = c.v2ReadyCheckComponent(ctx, t)
					} else {
						found, err = c.v1ReadyCheckComponent(ctx, t)
					}
					if found {
						if err != nil {
							return err
						}
						return nil
					}

					// fallback to http ready check
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
