package pyroscope

import (
	"github.com/grafana/dskit/services"

	"github.com/grafana/pyroscope/pkg/featureflags"
)

func (c *Config) getFeatureFlags() map[string]bool {
	return map[string]bool{
		featureflags.V2StorageLayer:          c.v2Experiment,
		featureflags.PyroscopeRuler:          c.CompactionWorker.MetricsExporter.Enabled,
		featureflags.PyroscopeRulerFunctions: false, // not supported yet
		featureflags.UTF8LabelNames:          false, // not supported yet
	}
}

func (f *Phlare) initFeatureFlags() (services.Service, error) {
	ff := featureflags.NewFromEnabled(f.reg, f.Cfg.getFeatureFlags())
	f.API.RegisterFeatureFlagsServiceHandler(ff)
	return nil, nil
}
