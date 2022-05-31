package fire

import (
	"flag"

	"github.com/grafana/dskit/flagext"

	"github.com/grafana/fire/pkg/agent"
)

type Config struct {
	Target        flagext.StringSliceCSV `yaml:"target,omitempty"`
	ScrapeConfigs []*agent.ScrapeConfig  `yaml:"scrape_configs,omitempty"`
}

// RegisterFlags registers flag.
func (c *Config) RegisterFlags(f *flag.FlagSet) {
	// Set the default module list to 'all'
	c.Target = []string{All}
	f.Var(&c.Target, "target", "Comma-separated list of Loki modules to load. "+
		"The alias 'all' can be used in the list to load a number of core modules and will enable single-binary mode. ")
}
