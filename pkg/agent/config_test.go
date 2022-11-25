package agent

import (
	"strings"
	"testing"

	parcaconfig "github.com/parca-dev/parca/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	cfg := &ScrapeConfig{
		JobName: "foo",
		ProfilingConfig: &parcaconfig.ProfilingConfig{
			PprofPrefix: "/prefix",
		},
	}

	require.NoError(t, cfg.Validate())

	require.Equal(t, len(parcaconfig.DefaultScrapeConfig().ProfilingConfig.PprofConfig), len(cfg.ProfilingConfig.PprofConfig))
	for _, p := range cfg.ProfilingConfig.PprofConfig {
		require.True(t, strings.HasPrefix(p.Path, "/prefix"))
	}
}
