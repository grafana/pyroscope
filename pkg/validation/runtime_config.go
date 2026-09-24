package validation

import (
	"fmt"
	"io"
	"time"

	"github.com/go-kit/log/level"
	"go.yaml.in/yaml/v3"

	"github.com/grafana/pyroscope/v2/pkg/profiledump"
	"github.com/grafana/pyroscope/v2/pkg/util"
)

type RuntimeConfigValues struct {
	TenantLimits map[string]*Limits `yaml:"overrides"`
}

func (r RuntimeConfigValues) validate(bounds profiledump.Config, now time.Time) error {
	if err := bounds.Validate(); err != nil {
		return fmt.Errorf("profile_dump: %w", err)
	}
	for t, c := range r.TenantLimits {
		if c == nil {
			level.Warn(util.Logger).Log("msg", "skipping empty tenant limit definition", "tenant", t)
			continue
		}

		if err := c.Validate(bounds, now); err != nil {
			return fmt.Errorf("invalid override for tenant %s: %w", t, err)
		}
	}

	return nil
}

// LoadRuntimeConfig uses the provisional default process bounds.
func LoadRuntimeConfig(r io.Reader) (*RuntimeConfigValues, error) {
	return LoadRuntimeConfigWithProfileDump(r, profiledump.DefaultConfig(), time.Now())
}

// LoadRuntimeConfigWithProfileDump validates a complete reload with process-wide
// bounds and a single clock reading, before the runtime manager publishes it.
func LoadRuntimeConfigWithProfileDump(r io.Reader, bounds profiledump.Config, now time.Time) (*RuntimeConfigValues, error) {
	overrides := &RuntimeConfigValues{}

	decoder := yaml.NewDecoder(r)
	decoder.KnownFields(true)
	if err := decoder.Decode(&overrides); err != nil {
		return nil, err
	}
	if err := overrides.validate(bounds, now); err != nil {
		return nil, err
	}
	return overrides, nil
}
