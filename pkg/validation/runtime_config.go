package validation

import (
	"fmt"
	"io"
	"time"

	"github.com/go-kit/log/level"
	"go.yaml.in/yaml/v3"

	"github.com/grafana/pyroscope/v2/pkg/util"
)

type RuntimeConfigValues struct {
	TenantLimits map[string]*Limits `yaml:"overrides"`
}

func (r RuntimeConfigValues) validate(now time.Time) error {
	for t, c := range r.TenantLimits {
		if c == nil {
			level.Warn(util.Logger).Log("msg", "skipping empty tenant limit definition", "tenant", t)
			continue
		}

		if err := c.validate(now); err != nil {
			return fmt.Errorf("invalid override for tenant %s: %w", t, err)
		}
	}

	return nil
}

// LoadRuntimeConfig validates a complete reload before publication.
func LoadRuntimeConfig(r io.Reader) (*RuntimeConfigValues, error) {
	return loadRuntimeConfig(r, time.Now())
}

func loadRuntimeConfig(r io.Reader, now time.Time) (*RuntimeConfigValues, error) {
	overrides := &RuntimeConfigValues{}

	decoder := yaml.NewDecoder(r)
	decoder.KnownFields(true)
	if err := decoder.Decode(&overrides); err != nil {
		return nil, err
	}
	if err := overrides.validate(now); err != nil {
		return nil, err
	}
	return overrides, nil
}
