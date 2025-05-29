package phlare

import (
	"github.com/grafana/dskit/services"

	"github.com/grafana/pyroscope/pkg/featureflags"
)

func (c *Config) getFeatureFlags() (*featureflags.FeatureFlags, error) {
	ff, err := featureflags.NewFromEnabled(map[string]bool{
		featureflags.V2StorageLayer:          c.v2Experiment,
		featureflags.PyroscopeRuler:          c.CompactionWorker.MetricsExporter.Enabled,
		featureflags.PyroscopeRulerFunctions: false, // not supported yet
		featureflags.UTF8LabelNames:          false, // not supported yet
	})
	if err != nil {
		return nil, err
	}
	return ff, nil
}

func (f *Phlare) initFeatureFlags() (services.Service, error) {
	ff, err := f.Cfg.getFeatureFlags()
	if err != nil {
		return nil, err
	}

	f.API.RegisterFeatureFlagsServiceHandler(ff)
	return nil, nil
}
