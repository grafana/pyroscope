package integration

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/cfg"
	"github.com/grafana/pyroscope/pkg/phlare"
)

type PyroscopeTest struct {
	config phlare.Config
	it     *phlare.Phlare
	wg     sync.WaitGroup
	reg    prometheus.Registerer
}

func (p *PyroscopeTest) Start(t *testing.T) {

	p.reg = prometheus.DefaultRegisterer
	prometheus.DefaultRegisterer = prometheus.NewRegistry()

	err := cfg.DynamicUnmarshal(&p.config, []string{"pyroscope"}, flag.NewFlagSet("pyroscope", flag.ContinueOnError))
	require.NoError(t, err)
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
	return httpBodyContains("http://localhost:4040/ready", "ready")
}
func (p *PyroscopeTest) ringActive() bool {
	return httpBodyContains("http://localhost:4040/ring", "ACTIVE")
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
