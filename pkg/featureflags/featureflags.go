package featureflags

import (
	"context"
	"fmt"
	"slices"

	"connectrpc.com/connect"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

const (
	V2StorageLayer          = "v2StorageLayer"
	PyroscopeRuler          = "pyroscopeRuler"
	PyroscopeRulerFunctions = "pyroscopeRulerFunctions"
	UTF8LabelNames          = "utf8LabelNames"
)

type Scope string

const (
	ScopePusherV1  Scope = "pusherV1"  // this feature flag is relevant for the pusher.v1 API.
	ScopeQuerierV1 Scope = "querierV1" // this feature flag is relevant for the querier.v1 API.
)

type featureDependency struct {
	requires  []string // the feature cannot work without this other feature flag enabled.
	conflicts []string // the feature cannot work with this other feature flag enabled.
}

type featureFlag struct {
	Parameters   typesv1.FeatureParameters
	dependencies featureDependency
	apiScopes    []Scope
	isStatic     bool // if true, the feature flag is static and determined by the configuration file for all tenants.
}

func (f *featureFlag) Clone() *featureFlag {
	return &featureFlag{
		Parameters:   *f.Parameters.CloneVT(),
		dependencies: f.dependencies,
		apiScopes:    f.apiScopes,
		isStatic:     f.isStatic,
	}
}
func stringPtr(s string) *string {
	return &s
}

var (
	allFeatureFlags = map[string]*featureFlag{
		V2StorageLayer: {
			Parameters: typesv1.FeatureParameters{
				Enabled:          false,
				Description:      stringPtr("v2 storage layer"),
				DocumentationUrl: stringPtr("https://github.com/grafana/pyroscope/blob/2b8e08ac9098113d3ba3376b5ae53e81ef8511ec/pkg/experiment/README.md"),
			},
			isStatic: true,
		},
		PyroscopeRuler: {
			Parameters: typesv1.FeatureParameters{
				Enabled:     false,
				Description: stringPtr("Supports the Pyroscope ruler, which exports Prometheus metrics from Profiling Data."),
			},
			dependencies: featureDependency{
				requires: []string{V2StorageLayer},
			},
			apiScopes: []Scope{ScopeQuerierV1},
		},
		PyroscopeRulerFunctions: {
			Parameters: typesv1.FeatureParameters{
				Enabled:     false,
				Description: stringPtr("Enables function support for the Pyroscope ruler, which allows to export resource usage on a per function level."),
			},
			dependencies: featureDependency{
				requires: []string{PyroscopeRuler},
			},
			apiScopes: []Scope{ScopeQuerierV1},
		},
		UTF8LabelNames: {
			Parameters: typesv1.FeatureParameters{
				Enabled:     false,
				Description: stringPtr("Supports UTF-8 label names for Pyroscope read/write APIs."),
			},
			apiScopes: []Scope{ScopePusherV1, ScopeQuerierV1},
		},
	}
)

func applyFlags(allowStaticFlags bool, flagsSets ...map[string]bool) (map[string]*featureFlag, error) {
	// reduce the flags sets to a single map
	flags := make(map[string]bool)
	for _, flagSet := range flagsSets {
		for name, flag := range flagSet {
			flags[name] = flag
		}
	}

	result := make(map[string]*featureFlag)

	// apply the flags to the feature flags
	for name, flag := range allFeatureFlags {
		flag = flag.Clone()
		if !allowStaticFlags && flag.isStatic {
			if _, ok := flags[name]; ok {
				return nil, fmt.Errorf("feature flag %s is a static flag and cannot be changed", name)
			}
		}

		if _, ok := flags[name]; ok {
			flag.Parameters.Enabled = flags[name]
			delete(flags, name)
		}

		result[name] = flag
	}

	for name := range flags {
		return nil, fmt.Errorf("feature flag %s is not defined", name)
	}

	return result, nil
}

type FeatureFlags struct {
	flags map[string]*featureFlag
}

type Handler interface {
	GetFeatureFlags(ctx context.Context, req *connect.Request[typesv1.GetFeatureFlagsRequest]) (*connect.Response[typesv1.GetFeatureFlagsResponse], error)
}

func (f *FeatureFlags) validate() error {
	// TODO: check for cycle and dependencies
	return nil
}

func (f *FeatureFlags) HandlerForScope(scope Scope) Handler {
	flags := make(map[string]*typesv1.FeatureParameters)
	for name, flag := range f.flags {
		if !slices.Contains(flag.apiScopes, scope) {
			continue
		}
		if !flag.Parameters.Enabled {
			continue
		}
		flags[name] = flag.Parameters.CloneVT()
	}

	return &handler{flags: flags}
}

type handler struct {
	flags map[string]*typesv1.FeatureParameters
}

func (h *handler) GetFeatureFlags(
	ctx context.Context,
	req *connect.Request[typesv1.GetFeatureFlagsRequest],
) (*connect.Response[typesv1.GetFeatureFlagsResponse], error) {
	return &connect.Response[typesv1.GetFeatureFlagsResponse]{
		Msg: &typesv1.GetFeatureFlagsResponse{
			Features: h.flags,
		},
	}, nil
}

func NewFromEnabled(enabled map[string]bool) (*FeatureFlags, error) {
	flags, err := applyFlags(true, enabled)
	if err != nil {
		return nil, err
	}

	result := &FeatureFlags{flags: flags}
	if err := result.validate(); err != nil {
		return nil, err
	}

	return result, nil
}
