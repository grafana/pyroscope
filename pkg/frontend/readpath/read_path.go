package readpath

import (
	"flag"
)

type Config struct {
	EnableQueryBackend       bool             `yaml:"enable_query_backend" json:"enable_query_backend" category:"advanced" doc:"hidden"`
	EnableQueryBackendFrom   QueryBackendFrom `yaml:"enable_query_backend_from" json:"enable_query_backend_from" doc:"hidden"`
	QueryTreeEnabled         bool             `yaml:"query_tree_enabled" json:"query_tree_enabled" category:"experimental" doc:"hidden"`
	ForceDatasetIndexLookup  bool             `yaml:"force_dataset_index_lookup" json:"force_dataset_index_lookup" category:"experimental" doc:"hidden"`
	DatasetIndexShardPruning bool             `yaml:"dataset_index_shard_pruning" json:"dataset_index_shard_pruning" category:"experimental" doc:"hidden"`
}

func (o *Config) RegisterFlags(f *flag.FlagSet) {
	f.BoolVar(&o.EnableQueryBackend, "enable-query-backend", true,
		"This parameter specifies whether the new query backend is enabled.")
	o.EnableQueryBackendFrom = QueryBackendFrom{Auto: true}
	f.Var(&o.EnableQueryBackendFrom, "enable-query-backend-from",
		"This parameter specifies the point in time from which data is queried from the new query backend. "+
			"The value can be an RFC3339 timestamp (2020-10-20T00:00:00Z) or \"auto\" to automatically "+
			"determine the split point from the tenant's oldest profile time in the metastore.")
	f.BoolVar(&o.QueryTreeEnabled, "querier.query-tree-enabled", false,
		"Use the tree-based query path for SelectMergeProfile. Experimental.")
	f.BoolVar(&o.ForceDatasetIndexLookup, "querier.force-dataset-index-lookup", false,
		"Resolve the datasets to query from the dataset index stored in each block instead of the dataset entries in the metastore, regardless of the label selector.")
	f.BoolVar(&o.DatasetIndexShardPruning, "querier.dataset-index-shard-pruning", false,
		"When the dataset index lookup is forced, send the service_name matcher to the metastore so that it skips shards that do not contain the service. Requires a metastore that supports it.")
}
