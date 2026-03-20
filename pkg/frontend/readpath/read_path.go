package readpath

import (
	"flag"
)

type Config struct {
	EnableQueryBackend     bool             `yaml:"enable_query_backend" json:"enable_query_backend" doc:"hidden"`
	EnableQueryBackendFrom QueryBackendFrom `yaml:"enable_query_backend_from" json:"enable_query_backend_from" doc:"hidden"`
	QueryTreeEnabled       bool             `yaml:"query_tree_enabled" json:"query_tree_enabled" doc:"hidden"`
}

func (o *Config) RegisterFlags(f *flag.FlagSet) {
	f.BoolVar(&o.EnableQueryBackend, "enable-query-backend", false,
		"This parameter specifies whether the new query backend is enabled.")
	f.Var(&o.EnableQueryBackendFrom, "enable-query-backend-from",
		"This parameter specifies the point in time from which data is queried from the new query backend. "+
			"The value can be an RFC3339 timestamp (2020-10-20T00:00:00Z) or \"auto\" to automatically "+
			"determine the split point from the tenant's oldest profile time in the metastore.")
	f.BoolVar(&o.QueryTreeEnabled, "querier.query-tree-enabled", false,
		"Use the tree-based query path for SelectMergeProfile. Experimental.")
}
