package agent

import (
	"fmt"
	"net/url"
	"time"

	"github.com/parca-dev/parca/pkg/config"
	parcaconfig "github.com/parca-dev/parca/pkg/config"
	commonconfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/kubernetes"
	"github.com/prometheus/prometheus/model/relabel"
)

type Config struct {
	ScrapeConfigs []*ScrapeConfig `yaml:"scrape_configs,omitempty"`
}

func (c *Config) Validate() error {
	for _, cfg := range c.ScrapeConfigs {
		if err := cfg.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type ScrapeConfig struct {
	JobName                string                       `yaml:"job_name"`
	Params                 url.Values                   `yaml:"params,omitempty"`
	ScrapeInterval         model.Duration               `yaml:"scrape_interval,omitempty"`
	ScrapeTimeout          model.Duration               `yaml:"scrape_timeout,omitempty"`
	Scheme                 string                       `yaml:"scheme,omitempty"`
	RelabelConfigs         []*relabel.Config            `yaml:"relabel_configs,omitempty"`
	ServiceDiscoveryConfig ServiceDiscoveryConfig       `yaml:",inline"`
	ProfilingConfig        *parcaconfig.ProfilingConfig `yaml:"profiling_config,omitempty"`

	HTTPClientConfig commonconfig.HTTPClientConfig `yaml:",inline"`
}

func (c *ScrapeConfig) Validate() error {
	defaults := config.DefaultScrapeConfig()
	if c.ScrapeInterval == 0 {
		c.ScrapeInterval = defaults.ScrapeInterval
	}
	if c.ScrapeTimeout == 0 {
		c.ScrapeTimeout = defaults.ScrapeTimeout
	}
	if c.Scheme == "" {
		c.Scheme = defaults.Scheme
	}
	if c.ProfilingConfig == nil || c.ProfilingConfig.PprofConfig == nil {
		c.ProfilingConfig = defaults.ProfilingConfig
	} else {
		for pt, pc := range defaults.ProfilingConfig.PprofConfig {
			if c.ProfilingConfig.PprofConfig[pt] == nil {
				c.ProfilingConfig.PprofConfig[pt] = pc
				continue
			}
			if c.ProfilingConfig.PprofConfig[pt].Enabled == nil {
				t := true
				c.ProfilingConfig.PprofConfig[pt].Enabled = &t
			}
			if c.ProfilingConfig.PprofConfig[pt].Path == "" {
				c.ProfilingConfig.PprofConfig[pt].Path = pc.Path
			}
		}
	}

	if c.JobName == "" {
		return fmt.Errorf("job_name is empty")
	}
	if c.ScrapeTimeout > c.ScrapeInterval {
		return fmt.Errorf("scrape timeout must be larger or equal to inverval for: %v", c.JobName)
	}
	if c.ScrapeTimeout == 0 {
		c.ScrapeTimeout = c.ScrapeInterval
	}

	if cfg, ok := c.ProfilingConfig.PprofConfig[pprofProcessCPU]; ok {
		if *cfg.Enabled && c.ScrapeTimeout < model.Duration(time.Second*2) {
			return fmt.Errorf("%v scrape_timeout must be at least 2 seconds in %v", pprofProcessCPU, c.JobName)
		}
	}
	return nil
}

type ServiceDiscoveryConfig struct {
	StaticConfigs       discovery.StaticConfig `yaml:"static_configs"`
	KubernetesSDConfigs []*kubernetes.SDConfig `yaml:"kubernetes_sd_configs,omitempty"`
}

func (cfg ServiceDiscoveryConfig) Configs() (res discovery.Configs) {
	if x := cfg.StaticConfigs; len(x) > 0 {
		res = append(res, x)
	}
	for _, x := range cfg.KubernetesSDConfigs {
		res = append(res, x)
	}
	return res
}
