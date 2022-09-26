package agent

import (
	"flag"
	"fmt"
	"net/url"
	"time"

	"github.com/grafana/dskit/flagext"
	"github.com/parca-dev/parca/pkg/config"
	parcaconfig "github.com/parca-dev/parca/pkg/config"
	commonconfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/kubernetes"
	"github.com/prometheus/prometheus/model/relabel"

	"github.com/grafana/fire/pkg/tenant"
)

type Config struct {
	ScrapeConfigs []*ScrapeConfig `yaml:"scrape_configs,omitempty"`
	ClientConfig  ClientConfig    `yaml:"client,omitempty"`
}

// RegisterFlags with prefix registers flags where every name is prefixed by
// prefix. If prefix is a non-empty string, prefix should end with a period.
func (c *ClientConfig) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.Var(&c.URL, prefix+"client.url", "URL of log server.")
	f.StringVar(&c.TenantID, prefix+"client.tenant-id", tenant.DefaultTenantID, "Tenant ID to use when pushing profiles to Fire (default: anonymous).")
	// Default backoff schedule: 0.5s, 1s, 2s, 4s, 8s, 16s, 32s, 64s, 128s, 256s(4.267m) For a total time of 511.5s(8.5m) before logs are lost
	// f.IntVar(&c.BackoffConfig.MaxRetries, prefix+"client.max-retries", MaxRetries, "Maximum number of retires when sending batches (deprecated).")
	// f.DurationVar(&c.BackoffConfig.MinBackoff, prefix+"client.min-backoff", MinBackoff, "Initial backoff time between retries (deprecated).")
	// f.DurationVar(&c.BackoffConfig.MaxBackoff, prefix+"client.max-backoff", MaxBackoff, "Maximum backoff time between retries (deprecated).")
}

// RegisterFlags registers flags.
func (c *Config) RegisterFlags(flags *flag.FlagSet) {
	c.ClientConfig.RegisterFlagsWithPrefix("", flags)
}

func (c *Config) Validate() error {
	for _, cfg := range c.ScrapeConfigs {
		if err := cfg.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type ClientConfig struct {
	URL       flagext.URLValue
	BatchWait time.Duration
	BatchSize int
	Client    commonconfig.HTTPClientConfig `yaml:",inline"`
	// The tenant ID to use when pushing profiles to Fire (default to anonymous).
	TenantID string `yaml:"tenant_id"`
	// todo add backoff config
	// BackoffConfig backoff.Config                `yaml:"backoff_config"`
}

func (c *ClientConfig) Validate() error {
	if c.URL.String() == "" {
		return fmt.Errorf("client: url is empty")
	}
	return c.Client.Validate()
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
