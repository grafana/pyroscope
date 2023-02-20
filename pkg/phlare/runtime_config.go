package phlare

import (
	"fmt"
	"io"
	"net/http"

	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/runtimeconfig"
	"gopkg.in/yaml.v2"

	"github.com/grafana/phlare/pkg/util"
	"github.com/grafana/phlare/pkg/validation"
)

type runtimeConfigValues struct {
	TenantLimits map[string]*validation.Limits `yaml:"overrides"`
}

func (r runtimeConfigValues) validate() error {
	for t, c := range r.TenantLimits {
		if c == nil {
			level.Warn(util.Logger).Log("msg", "skipping empty tenant limit definition", "tenant", t)
			continue
		}

		if err := c.Validate(); err != nil {
			return fmt.Errorf("invalid override for tenant %s: %w", t, err)
		}
	}
	return nil
}

func loadRuntimeConfig(r io.Reader) (interface{}, error) {
	overrides := &runtimeConfigValues{}

	decoder := yaml.NewDecoder(r)
	decoder.SetStrict(true)
	if err := decoder.Decode(&overrides); err != nil {
		return nil, err
	}
	if err := overrides.validate(); err != nil {
		return nil, err
	}
	return overrides, nil
}

type tenantLimitsFromRuntimeConfig struct {
	c *runtimeconfig.Manager
}

func (t *tenantLimitsFromRuntimeConfig) AllByTenantID() map[string]*validation.Limits {
	if t.c == nil {
		return nil
	}

	cfg, ok := t.c.GetConfig().(*runtimeConfigValues)
	if cfg != nil && ok {
		return cfg.TenantLimits
	}

	return nil
}

func (t *tenantLimitsFromRuntimeConfig) TenantLimits(userID string) *validation.Limits {
	allByUserID := t.AllByTenantID()
	if allByUserID == nil {
		return nil
	}

	return allByUserID[userID]
}

func newTenantLimits(c *runtimeconfig.Manager) validation.TenantLimits {
	return &tenantLimitsFromRuntimeConfig{c: c}
}

func runtimeConfigHandler(runtimeCfgManager *runtimeconfig.Manager, defaultLimits validation.Limits) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, ok := runtimeCfgManager.GetConfig().(*runtimeConfigValues)
		if !ok || cfg == nil {
			util.WriteTextResponse(w, "runtime config file doesn't exist")
			return
		}

		var output interface{}
		switch r.URL.Query().Get("mode") {
		case "diff":
			// Default runtime config is just empty struct, but to make diff work,
			// we set defaultLimits for every tenant that exists in runtime config.
			defaultCfg := runtimeConfigValues{}
			defaultCfg.TenantLimits = map[string]*validation.Limits{}
			for k, v := range cfg.TenantLimits {
				if v != nil {
					defaultCfg.TenantLimits[k] = &defaultLimits
				}
			}

			cfgYaml, err := util.YAMLMarshalUnmarshal(cfg)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			defaultCfgYaml, err := util.YAMLMarshalUnmarshal(defaultCfg)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			output, err = util.DiffConfig(defaultCfgYaml, cfgYaml)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

		default:
			output = cfg
		}
		util.WriteYAMLResponse(w, output)
	}
}
