package agent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/agent/scrape"
)

func TestValidate(t *testing.T) {
	cfg := &ScrapeConfig{
		JobName: "foo",
		ProfilingConfig: &scrape.ProfilingConfig{
			PprofPrefix: "/prefix",
		},
	}

	require.NoError(t, cfg.Validate())

	require.Equal(t, len(DefaultScrapeConfig().ProfilingConfig.PprofConfig), len(cfg.ProfilingConfig.PprofConfig))
	for _, p := range cfg.ProfilingConfig.PprofConfig {
		require.True(t, strings.HasPrefix(p.Path, "/prefix"))
	}
}
