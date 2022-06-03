// Copyright 2018 The Parca Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	commonconfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery"
	_ "github.com/prometheus/prometheus/discovery/install" // Imported for registration side-effect
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/relabel"
	"gopkg.in/yaml.v2"

	"github.com/parca-dev/parca/pkg/debuginfo"
)

const (
	pprofMemory     string = "memory"
	pprofBlock      string = "block"
	pprofGoroutine  string = "goroutine"
	pprofMutex      string = "mutex"
	pprofProcessCPU string = "process_cpu"
)

// Config holds all the configuration information for Parca.
type Config struct {
	DebugInfo     *debuginfo.Config `yaml:"debug_info"`
	ScrapeConfigs []*ScrapeConfig   `yaml:"scrape_configs,omitempty"`
}

// Validate returns an error if the config is not valid.
func (c *Config) Validate() error {
	return validation.ValidateStruct(c,
		validation.Field(&c.DebugInfo, validation.Required, debuginfo.Valid),
	)
}

func trueValue() *bool {
	a := true
	return &a
}

func DefaultScrapeConfig() ScrapeConfig {
	return ScrapeConfig{
		ScrapeInterval: model.Duration(time.Second * 10),
		ScrapeTimeout:  model.Duration(time.Second * 0),
		Scheme:         "http",
		ProfilingConfig: &ProfilingConfig{
			PprofConfig: PprofConfig{
				pprofMemory: &PprofProfilingConfig{
					Enabled: trueValue(),
					Path:    "/debug/pprof/allocs",
				},
				pprofBlock: &PprofProfilingConfig{
					Enabled: trueValue(),
					Path:    "/debug/pprof/block",
				},
				pprofGoroutine: &PprofProfilingConfig{
					Enabled: trueValue(),
					Path:    "/debug/pprof/goroutine",
				},
				pprofMutex: &PprofProfilingConfig{
					Enabled: trueValue(),
					Path:    "/debug/pprof/mutex",
				},
				pprofProcessCPU: &PprofProfilingConfig{
					Enabled: trueValue(),
					Delta:   true,
					Path:    "/debug/pprof/profile",
				},
			},
		},
	}
}

func (c Config) String() string {
	b, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Sprintf("<error creating config string: %s>", err)
	}
	return string(b)
}

// SetDirectory joins any relative file paths with dir.
func (c *Config) SetDirectory(dir string) {
	for _, c := range c.ScrapeConfigs {
		c.SetDirectory(dir)
	}
}

// Load parses the YAML input s into a Config.
func Load(s string) (*Config, error) {
	cfg := &Config{}

	err := yaml.UnmarshalStrict([]byte(s), cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// LoadFile parses the given YAML file into a Config.
func LoadFile(filename string) (*Config, error) {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	cfg, err := Load(string(content))
	if err != nil {
		return nil, fmt.Errorf("parsing YAML file %s: %v", filename, err)
	}
	cfg.SetDirectory(filepath.Dir(filename))
	return cfg, nil
}

// ScrapeConfig configures a scraping unit for conprof.
type ScrapeConfig struct {
	// Name of the section in the config
	JobName string `yaml:"job_name,omitempty"`
	// A set of query parameters with which the target is scraped.
	Params url.Values `yaml:"params,omitempty"`
	// How frequently to scrape the targets of this scrape config.
	ScrapeInterval model.Duration `yaml:"scrape_interval,omitempty"`
	// The timeout for scraping targets of this config.
	ScrapeTimeout model.Duration `yaml:"scrape_timeout,omitempty"`
	// The URL scheme with which to fetch metrics from targets.
	Scheme string `yaml:"scheme,omitempty"`

	ProfilingConfig *ProfilingConfig `yaml:"profiling_config,omitempty"`

	RelabelConfigs []*relabel.Config `yaml:"relabel_configs,omitempty"`
	// We cannot do proper Go type embedding below as the parser will then parse
	// values arbitrarily into the overflow maps of further-down types.
	ServiceDiscoveryConfigs discovery.Configs             `yaml:"-"`
	HTTPClientConfig        commonconfig.HTTPClientConfig `yaml:",inline"`
}

// SetDirectory joins any relative file paths with dir.
func (c *ScrapeConfig) SetDirectory(dir string) {
	c.ServiceDiscoveryConfigs.SetDirectory(dir)
	c.HTTPClientConfig.SetDirectory(dir)
}

// ServiceDiscoveryConfig configures lists of different service discovery mechanisms.
type ServiceDiscoveryConfig struct {
	// List of labeled target groups for this job.
	StaticConfigs []*targetgroup.Group `yaml:"static_configs,omitempty"`
}

type ProfilingConfig struct {
	PprofConfig PprofConfig `yaml:"pprof_config,omitempty"`
}

type PprofConfig map[string]*PprofProfilingConfig

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *ScrapeConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	defaults := DefaultScrapeConfig()
	unmarshalled := ScrapeConfig{
		ScrapeInterval: defaults.ScrapeInterval,
		ScrapeTimeout:  defaults.ScrapeTimeout,
		Scheme:         defaults.Scheme,
	}
	if err := discovery.UnmarshalYAMLWithInlineConfigs(&unmarshalled, unmarshal); err != nil {
		return err
	}

	if unmarshalled.ProfilingConfig == nil || unmarshalled.ProfilingConfig.PprofConfig == nil {
		unmarshalled.ProfilingConfig = defaults.ProfilingConfig
	} else {
		// Merge unmarshalled config with defaults
		for pt, pc := range defaults.ProfilingConfig.PprofConfig {
			// nothing set yet so simply use the default
			if unmarshalled.ProfilingConfig.PprofConfig[pt] == nil {
				unmarshalled.ProfilingConfig.PprofConfig[pt] = pc
				continue
			}
			if unmarshalled.ProfilingConfig.PprofConfig[pt].Enabled == nil {
				unmarshalled.ProfilingConfig.PprofConfig[pt].Enabled = trueValue()
			}
			if unmarshalled.ProfilingConfig.PprofConfig[pt].Path == "" {
				unmarshalled.ProfilingConfig.PprofConfig[pt].Path = pc.Path
			}
		}
	}

	*c = unmarshalled

	if len(c.JobName) == 0 {
		return errors.New("job_name is empty")
	}

	// The UnmarshalYAML method of HTTPClientConfig is not being called because it's not a pointer.
	// We cannot make it a pointer as the parser panics for inlined pointer structs.
	// Thus we just do its validation here.
	if err := c.HTTPClientConfig.Validate(); err != nil {
		return err
	}

	// Check for users putting URLs in target groups.
	if len(c.RelabelConfigs) == 0 {
		if err := checkStaticTargets(c.ServiceDiscoveryConfigs); err != nil {
			return err
		}
	}

	for _, rlcfg := range c.RelabelConfigs {
		if rlcfg == nil {
			return errors.New("empty or null target relabeling rule in scrape config")
		}
	}

	// Validate the scrape and timeout internal configuration. When /debug/pprof/profile scraping
	// is enabled we need to make sure there is enough time to complete the scrape.
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

func checkStaticTargets(configs discovery.Configs) error {
	for _, cfg := range configs {
		sc, ok := cfg.(discovery.StaticConfig)
		if !ok {
			continue
		}
		for _, tg := range sc {
			for _, t := range tg.Targets {
				if err := CheckTargetAddress(t[model.AddressLabel]); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

type PprofProfilingConfig struct {
	Enabled *bool  `yaml:"enabled,omitempty"`
	Path    string `yaml:"path,omitempty"`
	Delta   bool   `yaml:"delta,omitempty"`
}

// CheckTargetAddress checks if target address is valid.
func CheckTargetAddress(address model.LabelValue) error {
	// For now check for a URL, we may want to expand this later.
	if strings.Contains(string(address), "/") {
		return fmt.Errorf("%q is not a valid hostname", address)
	}
	return nil
}
