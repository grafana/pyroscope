package cluster

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/pkg/cfg"
	"github.com/grafana/pyroscope/pkg/pyroscope"
)

type Component struct {
	Target  string
	replica int
	flags   []string
	cfg     pyroscope.Config
	p       *pyroscope.Pyroscope
	reg     *prometheus.Registry

	httpPort       int
	grpcPort       int
	memberlistPort int
	raftPort       int
}

func (c *Component) addPorts(ports []int) {
	if len(ports) < 1 {
		return
	}
	c.httpPort = ports[0]
	if len(ports) < 2 {
		return
	}
	c.grpcPort = ports[1]
	if len(ports) < 3 {
		return
	}
	c.memberlistPort = ports[2]
	if len(ports) < 4 {
		return
	}
	c.raftPort = ports[3]
}

func (comp *Component) querierReadyCheck(ctx context.Context, expectedIngesters, expectedStoreGateways int) (err error) {
	check := comp.checkMetrics().
		addExpectValue(float64(expectedIngesters), "pyroscope_ring_members", "name", "ingester", "state", "ACTIVE").
		addExpectValue(float64(expectedStoreGateways), "pyroscope_ring_members", "name", "store-gateway-client", "state", "ACTIVE")
	return check.run(ctx)
}

func (comp *Component) distributorReadyCheck(ctx context.Context, expectedIngesters, expectedDistributors, expectedSegmentWriters int) (err error) {
	check := comp.checkMetrics()
	if expectedIngesters > 0 {
		check = check.addExpectValue(float64(expectedIngesters), "pyroscope_ring_members", "name", "ingester", "state", "ACTIVE")
	}
	if expectedSegmentWriters > 0 {
		check = check.addExpectValue(float64(expectedSegmentWriters), "pyroscope_ring_members", "name", "segment-writer", "state", "ACTIVE")
	}
	if expectedDistributors > 0 {
		check = check.addExpectValue(float64(expectedDistributors), "pyroscope_ring_members", "name", "distributor", "state", "ACTIVE")
	}
	return check.run(ctx)
}

func (comp *Component) httpReadyCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://%s:%d/ready", listenAddr, comp.httpPort), nil)
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
	return fmt.Sprintf("[%s] http=%d grpc=%d memberlist=%d raft=%d", comp.nodeName(), comp.httpPort, comp.grpcPort, comp.memberlistPort, comp.raftPort)
}

func (comp *Component) nodeName() string {
	return fmt.Sprintf("%s-%d", comp.Target, comp.replica)
}

var lockRegistry sync.Mutex

func (comp *Component) start(_ context.Context) (*pyroscope.Pyroscope, error) {
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
	f, err := pyroscope.New(comp.cfg)
	if err != nil {
		return nil, err
	}

	return f, nil
}
