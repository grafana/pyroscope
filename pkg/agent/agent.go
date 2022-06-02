package agent

import (
	"net/url"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/model/relabel"
)

type Config struct {
	ScrapeConfigs []*ScrapeConfig `yaml:"scrape_configs,omitempty"`
}

type ScrapeConfig struct {
	JobName                string                 `yaml:"job_name"`
	Params                 url.Values             `yaml:"params,omitempty"`
	ScrapeInterval         model.Duration         `yaml:"scrape_interval,omitempty"`
	ScrapeTimeout          model.Duration         `yaml:"scrape_timeout,omitempty"`
	Scheme                 string                 `yaml:"scheme,omitempty"`
	RelabelConfigs         []*relabel.Config      `yaml:"relabel_configs,omitempty"`
	ServiceDiscoveryConfig ServiceDiscoveryConfig `yaml:",inline"`
}

type ServiceDiscoveryConfig struct {
	StaticConfigs discovery.StaticConfig `yaml:"static_configs"`
}
