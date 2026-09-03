package validation

import (
	"fmt"
	"io"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"go.yaml.in/yaml/v3"
)

type RuntimeConfigValues struct {
	TenantLimits map[string]*Limits `yaml:"overrides"`
}

func (r RuntimeConfigValues) validate(logger log.Logger) error {
	for t, c := range r.TenantLimits {
		if c == nil {
			level.Warn(logger).Log("msg", "skipping empty tenant limit definition", "tenant", t)
			continue
		}

		if err := c.Validate(); err != nil {
			return fmt.Errorf("invalid override for tenant %s: %w", t, err)
		}
	}

	return nil
}

func LoadRuntimeConfig(r io.Reader, logger log.Logger) (*RuntimeConfigValues, error) {
	overrides := &RuntimeConfigValues{}

	decoder := yaml.NewDecoder(r)
	decoder.KnownFields(true)
	if err := decoder.Decode(&overrides); err != nil {
		return nil, err
	}
	if err := overrides.validate(logger); err != nil {
		return nil, err
	}
	return overrides, nil
}
