package validation

import (
	"flag"
	"time"
)

type Symbolizer struct {
	// Enabled enables the symbolizer in the query frontend.
	Enabled bool `yaml:"enabled" json:"enabled" category:"experimental" doc:"hidden"`

	// Maximum symbol size is checked against both the payload size and the decompressed size
	MaxSymbolSizeBytes int `category:"advanced"`

	// SymbolRefTreesEnabled makes tree queries carry unresolved symbol
	// references through the native tree path instead of being rewritten to
	// pprof for symbolization; the frontend resolves references itself after
	// the final merge. Requires a symbolizer to be configured.
	SymbolRefTreesEnabled bool `yaml:"symbol_ref_trees_enabled" json:"symbol_ref_trees_enabled" category:"experimental" doc:"hidden"`

	// ResolveTimeout bounds how long the frontend waits to resolve a single
	// binary's unresolved addresses for a symbol-ref tree query. A binary
	// that exceeds this timebox falls back to binary!0xaddr frames for that
	// query only.
	ResolveTimeout time.Duration `yaml:"resolve_timeout" json:"resolve_timeout" category:"advanced"`

	// MaxUnresolvedLocations bounds the distinct unresolved locations a
	// single symbol-ref tree query may carry through the read path; a query
	// exceeding it fails rather than degrading.
	MaxUnresolvedLocations int `yaml:"max_unresolved_locations" json:"max_unresolved_locations" category:"experimental" doc:"hidden"`
}

func (s *Symbolizer) RegisterFlags(f *flag.FlagSet) {
	f.BoolVar(&s.Enabled, "symbolizer.enabled", false, "Enable symbolization for tenants by default.")
	f.IntVar(&s.MaxSymbolSizeBytes, "valdation.symbolizer.max-symbol-size-bytes", 512*1024*1024, "Maximum size of a symbol in bytes. This an upper limits to both the compressed and uncompressed size. 0 to disable.")
	f.BoolVar(&s.SymbolRefTreesEnabled, "symbolizer.symbol-ref-trees-enabled", false, "Enable symbol-aware tree references: tree queries are executed natively and symbolized by the frontend after the final merge, instead of being rewritten to pprof. Requires a symbolizer to be configured.")
	f.DurationVar(&s.ResolveTimeout, "symbolizer.resolve-timeout", 20*time.Second, "Maximum time the query frontend waits to resolve a single binary's unresolved addresses for a symbol-ref tree query, before falling back to binary!0xaddr frames for that binary.")
	f.IntVar(&s.MaxUnresolvedLocations, "symbolizer.max-unresolved-locations", 1_000_000, "Maximum number of distinct unresolved locations a symbol-ref tree query may carry through the read path before symbolization; a query exceeding the limit fails. 0 disables the limit.")
}

func (o *Overrides) SymbolizerEnabled(tenantID string) bool {
	return o.getOverridesForTenant(tenantID).Symbolizer.Enabled
}

func (o *Overrides) SymbolizerMaxSymbolSizeBytes(tenantID string) int {
	return o.getOverridesForTenant(tenantID).Symbolizer.MaxSymbolSizeBytes
}

func (o *Overrides) SymbolRefTreesEnabled(tenantID string) bool {
	return o.getOverridesForTenant(tenantID).Symbolizer.SymbolRefTreesEnabled
}

func (o *Overrides) SymbolizerResolveTimeout(tenantID string) time.Duration {
	return o.getOverridesForTenant(tenantID).Symbolizer.ResolveTimeout
}

func (o *Overrides) SymbolizerMaxUnresolvedLocations(tenantID string) int {
	return o.getOverridesForTenant(tenantID).Symbolizer.MaxUnresolvedLocations
}
