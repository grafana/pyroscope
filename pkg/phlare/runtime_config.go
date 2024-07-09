package phlare

import (
	"net/http"

	"github.com/grafana/dskit/runtimeconfig"

	"github.com/grafana/pyroscope/pkg/util"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
	"github.com/grafana/pyroscope/pkg/validation"
)

type tenantLimitsFromRuntimeConfig struct {
	c *runtimeconfig.Manager
}

func (t *tenantLimitsFromRuntimeConfig) AllByTenantID() map[string]*validation.Limits {
	if t.c == nil {
		return nil
	}

	cfg, ok := t.c.GetConfig().(*validation.RuntimeConfigValues)
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
		cfg, ok := runtimeCfgManager.GetConfig().(*validation.RuntimeConfigValues)
		if !ok || cfg == nil {
			util.WriteTextResponse(w, "runtime config file doesn't exist")
			return
		}

		var output interface{}
		switch r.URL.Query().Get("mode") {
		case "diff":
			// Default runtime config is just empty struct, but to make diff work,
			// we set defaultLimits for every tenant that exists in runtime config.
			defaultCfg := validation.RuntimeConfigValues{}
			defaultCfg.TenantLimits = map[string]*validation.Limits{}
			for k, v := range cfg.TenantLimits {
				if v != nil {
					defaultCfg.TenantLimits[k] = &defaultLimits
				}
			}

			cfgYaml, err := util.YAMLMarshalUnmarshal(cfg)
			if err != nil {
				httputil.Error(w, err)
				return
			}

			defaultCfgYaml, err := util.YAMLMarshalUnmarshal(defaultCfg)
			if err != nil {
				httputil.Error(w, err)
				return
			}

			output, err = util.DiffConfig(defaultCfgYaml, cfgYaml)
			if err != nil {
				httputil.Error(w, err)
				return
			}

		default:
			output = cfg
		}
		util.WriteYAMLResponse(w, output)
	}
}
