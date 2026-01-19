package validation

import (
	"flag"
)

type Symbolizer struct {
	// Enabled enables the symbolizer in the query frontend.
	Enabled bool `yaml:"enabled" json:"enabled" category:"experimental" doc:"hidden"`

	// Maximum symbol size is checked against both the payload size and the decompressed size
	MaxSymbolSizeBytes int
}

func (s *Symbolizer) RegisterFlags(f *flag.FlagSet) {
	f.BoolVar(&s.Enabled, "symbolizer.enabled", false, "Enable symbolization for tenants by default.")
	f.IntVar(&s.MaxSymbolSizeBytes, "valdation.symbolizer.max-symbol-size-bytes", 512*1024*1024, "Maximum size of a symbol in bytes. This an upper limits to both the compressed and uncompressed size. 0 to disable.")
}

func (o *Overrides) SymbolizerEnabled(tenantID string) bool {
	return o.getOverridesForTenant(tenantID).Symbolizer.Enabled
}

func (o *Overrides) SymbolizerMaxSymbolSizeBytes(tenantID string) int {
	return o.getOverridesForTenant(tenantID).Symbolizer.MaxSymbolSizeBytes
}
