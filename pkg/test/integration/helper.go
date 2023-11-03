package integration

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	"github.com/grafana/pyroscope/pkg/cfg"
	"github.com/grafana/pyroscope/pkg/phlare"
)

// getFreePorts returns a number of free local port for the tests to listen on. Note this will make sure the returned ports do not overlap, by stopping to listen once all ports are allocated
func getFreePorts(len int) (ports []int, err error) {
	ports = make([]int, len)
	for i := 0; i < len; i++ {
		var a *net.TCPAddr
		if a, err = net.ResolveTCPAddr("tcp", "localhost:0"); err == nil {
			var l *net.TCPListener
			if l, err = net.ListenTCP("tcp", a); err != nil {
				return nil, err
			}
			defer l.Close()
			ports[i] = l.Addr().(*net.TCPAddr).Port
		}
	}
	return ports, nil
}

type PyroscopeTest struct {
	config phlare.Config
	it     *phlare.Phlare
	wg     sync.WaitGroup
	reg    prometheus.Registerer

	httpPort       int
	memberlistPort int
}

const storeInMemory = "inmemory"

func (p *PyroscopeTest) Start(t *testing.T) {

	ports, err := getFreePorts(2)
	require.NoError(t, err)
	p.httpPort = ports[0]
	p.memberlistPort = ports[1]

	p.reg = prometheus.DefaultRegisterer
	prometheus.DefaultRegisterer = prometheus.NewRegistry()

	err = cfg.DynamicUnmarshal(&p.config, []string{"pyroscope"}, flag.NewFlagSet("pyroscope", flag.ContinueOnError))
	require.NoError(t, err)

	// set free ports
	p.config.Server.HTTPListenPort = p.httpPort
	p.config.MemberlistKV.AdvertisePort = p.memberlistPort
	p.config.MemberlistKV.TCPTransport.BindPort = p.memberlistPort

	// heartbeat more often
	p.config.Distributor.DistributorRing.HeartbeatPeriod = time.Second
	p.config.Ingester.LifecyclerConfig.HeartbeatPeriod = time.Second
	p.config.OverridesExporter.Ring.Ring.HeartbeatPeriod = time.Second
	p.config.QueryScheduler.ServiceDiscovery.SchedulerRing.HeartbeatPeriod = time.Second

	// do not use memberlist
	p.config.Distributor.DistributorRing.KVStore.Store = storeInMemory
	p.config.Ingester.LifecyclerConfig.RingConfig.KVStore.Store = storeInMemory
	p.config.OverridesExporter.Ring.Ring.KVStore.Store = storeInMemory
	p.config.QueryScheduler.ServiceDiscovery.SchedulerRing.KVStore.Store = storeInMemory

	p.config.SelfProfiling.DisablePush = true
	p.config.Analytics.Enabled = false // usage-stats terminating slow as hell
	p.config.LimitsConfig.MaxQueryLength = 0
	p.config.LimitsConfig.MaxQueryLookback = 0
	p.config.LimitsConfig.RejectOlderThan = 0
	_ = p.config.Server.LogLevel.Set("debug")
	p.it, err = phlare.New(p.config)

	require.NoError(t, err)

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		err := p.it.Run()
		require.NoError(t, err)
	}()
	require.Eventually(t, func() bool {
		return p.ringActive() && p.ready()
	}, 30*time.Second, 100*time.Millisecond)
}

func (p *PyroscopeTest) Stop(t *testing.T) {
	defer func() {
		prometheus.DefaultRegisterer = p.reg
	}()
	p.it.SignalHandler.Stop()
	p.wg.Wait()
}

func (p *PyroscopeTest) ready() bool {
	return httpBodyContains(p.URL()+"/ready", "ready")
}
func (p *PyroscopeTest) ringActive() bool {
	return httpBodyContains(p.URL()+"/ring", "ACTIVE")
}
func (p *PyroscopeTest) URL() string {
	return fmt.Sprintf("http://localhost:%d", p.httpPort)
}

func (p *PyroscopeTest) queryClient() querierv1connect.QuerierServiceClient {
	return querierv1connect.NewQuerierServiceClient(
		http.DefaultClient,
		p.URL(),
	)
}

func (p *PyroscopeTest) pushClient() pushv1connect.PusherServiceClient {
	return pushv1connect.NewPusherServiceClient(
		http.DefaultClient,
		p.URL(),
	)
}

func httpBodyContains(url string, needle string) bool {
	fmt.Println("httpBodyContains", url, needle)
	res, err := http.Get(url)
	if err != nil {
		return false
	}
	if res.StatusCode != 200 || res.Body == nil {
		return false
	}
	body := bytes.NewBuffer(nil)
	_, err = io.Copy(body, res.Body)
	if err != nil {
		return false
	}

	return strings.Contains(body.String(), needle)
}
