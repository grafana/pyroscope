package phlare

import (
	"github.com/grafana/pyroscope/pkg/featureflags"
)

func (c *Config) getFeatureFlags() (*featureflags.FeatureFlags, error) {
	ff, err := featureflags.NewFromEnabled(map[string]bool{
		featureflags.V2StorageLayer:          c.v2Experiment, // TODO: need to make sure its running v2 only for push and query
		featureflags.PyroscopeRuler:          c.CompactionWorker.MetricsExporter.Enabled,
		featureflags.PyroscopeRulerFunctions: false, // not supported yet
		featureflags.UTF8LabelNames:          false, // not supported yet
	})
	if err != nil {
		return nil, err
	}
	return ff, nil
}
