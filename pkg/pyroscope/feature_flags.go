package pyroscope

import (
	"github.com/grafana/dskit/services"

	"github.com/grafana/pyroscope/pkg/featureflags"
)

func (c *Config) getFeatureFlags() map[string]bool {
	rulerEnabled := c.CompactionWorker.MetricsExporter.Enabled
	return map[string]bool{
		featureflags.V2StorageLayer:          c.V2,
		featureflags.PyroscopeRuler:          rulerEnabled,
		featureflags.PyroscopeRulerFunctions: rulerEnabled,
		featureflags.UTF8LabelNames:          false, // not supported yet
	}
}

func (f *Pyroscope) initFeatureFlags() (services.Service, error) {
	ff := featureflags.NewFromEnabled(f.reg, f.Cfg.getFeatureFlags())
	f.API.RegisterFeatureFlagsServiceHandler(ff)
	return nil, nil
}
