package integration

import (
	"bytes"
	"flag"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/cfg"
	"github.com/grafana/pyroscope/pkg/phlare"
)

type PyroscopeTest struct {
	config phlare.Config
	it     *phlare.Phlare
	wg     sync.WaitGroup
}

func (p *PyroscopeTest) Start(t *testing.T) {

	err := cfg.DynamicUnmarshal(&p.config, []string{"pyroscope"}, flag.NewFlagSet("pyroscope", flag.ContinueOnError))
	require.NoError(t, err)
	p.config.SelfProfiling.DisablePush = true
	p.it, err = phlare.New(p.config)

	require.NoError(t, err)

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		err := p.it.Run()
		require.NoError(t, err)
	}()
	require.Eventually(t, func() bool {
		return p.ringActive()
	}, 5*time.Second, 100*time.Millisecond)
}

func (p *PyroscopeTest) Stop(t *testing.T) {
	p.it.SignalHandler.Stop()
	p.wg.Wait()
}

func (p *PyroscopeTest) ringActive() bool {
	res, err := http.Get("http://localhost:4040/ring")
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
	if strings.Contains(body.String(), "ACTIVE") {
		return true
	}
	return false
}
