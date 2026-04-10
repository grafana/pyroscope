package readpath

import (
	"flag"
)

type Config struct {
	EnableQuerier      bool        `yaml:"enable_querier" json:"enable_querier" doc:"hidden"`
	EnableQuerierUntil QuerierFrom `yaml:"enable_querier_until" json:"enable_querier_until" doc:"hidden"` // TODO
	QueryTreeEnabled   bool        `yaml:"query_tree_enabled" json:"query_tree_enabled" doc:"hidden"`
}

func (o *Config) RegisterFlags(f *flag.FlagSet) {
	f.BoolVar(&o.EnableQuerier, "enable-querier", false,
		"This parameter specifies whether the old querier is enabled.")
	f.Var(&o.EnableQuerierUntil, "enable-querier-until",
		"This parameter specifies the point in time until which data is queried from the old querier. "+
			"The value can be an RFC3339 timestamp (2020-10-20T00:00:00Z) or \"auto\" to automatically "+
			"determine the split point from the tenant's oldest profile time in the metastore.")
	f.BoolVar(&o.QueryTreeEnabled, "querier.query-tree-enabled", false,
		"Use the tree-based query path for SelectMergeProfile. Experimental.")
}
