package sampling

type Config struct {
	// UsageGroups controls sampling for pre-configured usage groups.
	UsageGroups map[string]UsageGroupSampling `yaml:"usage_groups" json:"usage_groups"`
}

type UsageGroupSampling struct {
	Probability float64 `yaml:"probability" json:"probability"`
}

type Source struct {
	UsageGroup  string  `json:"usageGroup"`
	Probability float64 `json:"probability"`
}
