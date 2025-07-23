package featureflags

import (
	"context"
	"sort"

	"connectrpc.com/connect"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	capabilitiesv1 "github.com/grafana/pyroscope/api/gen/proto/go/capabilities/v1"
)

const (
	V2StorageLayer          = "v2StorageLayer"
	PyroscopeRuler          = "pyroscopeRuler"
	PyroscopeRulerFunctions = "pyroscopeRulerFunctions"
	UTF8LabelNames          = "utf8LabelNames"
)

func stringPtr(s string) *string {
	return &s
}

var (
	allFeatureFlags = map[string]*capabilitiesv1.FeatureFlag{
		V2StorageLayer: {
			Enabled:          false,
			Description:      stringPtr("v2 storage layer"),
			DocumentationUrl: stringPtr("https://github.com/grafana/pyroscope/blob/main/pkg/pyroscope/PYROSCOPE_V2.md"),
		},
		PyroscopeRuler: {
			Enabled:     false,
			Description: stringPtr("Supports the Pyroscope ruler, which exports Prometheus metrics from Profiling Data."),
		},
		PyroscopeRulerFunctions: {
			Enabled:     false,
			Description: stringPtr("Enables function support for the Pyroscope ruler, which allows to export resource usage on a per function level."),
		},
		UTF8LabelNames: {
			Enabled:     false,
			Description: stringPtr("Supports UTF-8 label names for Pyroscope read/write APIs."),
		},
	}
)

type FeatureFlags struct {
	flags      []*capabilitiesv1.FeatureFlag
	infoMetric *prometheus.GaugeVec
}

func (h *FeatureFlags) GetFeatureFlags(
	ctx context.Context,
	req *connect.Request[capabilitiesv1.GetFeatureFlagsRequest],
) (*connect.Response[capabilitiesv1.GetFeatureFlagsResponse], error) {
	return &connect.Response[capabilitiesv1.GetFeatureFlagsResponse]{
		Msg: &capabilitiesv1.GetFeatureFlagsResponse{
			FeatureFlags: h.flags,
		},
	}, nil
}

func boolToFloat64(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}

func NewFromEnabled(reg prometheus.Registerer, enabled map[string]bool) *FeatureFlags {
	ff := &FeatureFlags{
		infoMetric: promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Name: "pyroscope_feature_flags_enabled",
			Help: "Shows the global feature flags",
		}, []string{"name"}),
	}

	ff.flags = make([]*capabilitiesv1.FeatureFlag, 0, len(allFeatureFlags))
	for name, flag := range allFeatureFlags {
		flag := flag.CloneVT()
		flag.Name = name
		ff.flags = append(ff.flags, flag)
		flag.Enabled = enabled[name]
		enabled[name] = true
		ff.infoMetric.WithLabelValues(flag.Name).Set(boolToFloat64(flag.Enabled))
	}
	sort.Slice(ff.flags, func(i, j int) bool {
		return ff.flags[i].Name < ff.flags[j].Name

	})

	return ff
}
