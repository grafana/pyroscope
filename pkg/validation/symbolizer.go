package validation

import (
	"flag"
)

type Symbolizer struct {
	// Enabled enables the symbolizer in the query frontend.
	Enabled bool `yaml:"enabled" json:"enabled" category:"experimental" doc:"hidden"`
}

func (s *Symbolizer) RegisterFlags(f *flag.FlagSet) {
	f.BoolVar(&s.Enabled, "symbolizer.enabled", false, "Enable symbolization for tenants by default.")
}

func (o *Overrides) SymbolizerEnabled(tenantID string) bool {
	return o.getOverridesForTenant(tenantID).Symbolizer.Enabled
}
