package validation

import (
	"fmt"
	"io"
	"math"

	"github.com/go-kit/log/level"
	"go.yaml.in/yaml/v3"

	"github.com/grafana/pyroscope/v2/pkg/profiledump"
	"github.com/grafana/pyroscope/v2/pkg/util"
)

type RuntimeConfigValues struct {
	TenantLimits map[string]*Limits `yaml:"overrides"`
}

func (r RuntimeConfigValues) validate(processRate float64) error {
	if processRate <= 0 || math.IsNaN(processRate) || math.IsInf(processRate, 0) {
		return fmt.Errorf("profile_dump: process_captures_per_second must be finite and positive")
	}
	for t, c := range r.TenantLimits {
		if c == nil {
			level.Warn(util.Logger).Log("msg", "skipping empty tenant limit definition", "tenant", t)
			continue
		}

		if err := c.Validate(processRate); err != nil {
			return fmt.Errorf("invalid override for tenant %s: %w", t, err)
		}
	}

	return nil
}

// LoadRuntimeConfig uses the provisional default process rate.
func LoadRuntimeConfig(r io.Reader) (*RuntimeConfigValues, error) {
	return LoadRuntimeConfigWithProfileDump(r, profiledump.DefaultRecorderConfig().ProcessCapturesPerSecond)
}

// LoadRuntimeConfigWithProfileDump validates a complete reload before publication.
// The process rate supplies the default for omitted tenant capture rates.
func LoadRuntimeConfigWithProfileDump(r io.Reader, processRate float64) (*RuntimeConfigValues, error) {
	overrides := &RuntimeConfigValues{}

	decoder := yaml.NewDecoder(r)
	decoder.KnownFields(true)
	if err := decoder.Decode(&overrides); err != nil {
		return nil, err
	}
	if err := overrides.validate(processRate); err != nil {
		return nil, err
	}
	return overrides, nil
}
