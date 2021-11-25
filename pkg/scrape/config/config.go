// Copyright 2013 The Prometheus Authors
// Copyright 2021 The Pyroscope Authors
//
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
	"net/url"
	"strings"
	"time"

	"github.com/imdario/mergo"

	"github.com/pyroscope-io/pyroscope/pkg/scrape/discovery"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/relabel"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

// revive:disable:max-public-structs complex domain

// DefaultConfig returns the default scrape configuration.
func DefaultConfig() *Config {
	return &Config{
		ScrapeInterval: 10 * time.Second,
		ScrapeTimeout:  15 * time.Second,

		Profiles: map[string]*Profile{
			"cpu": {
				Path: "/debug/pprof/profile",
				Params: url.Values{
					"seconds": []string{"10"},
				},
				SampleTypes: map[string]*SampleTypeConfig{
					"samples": {
						DisplayName: "cpu",
						Units:       "samples",
						Sampled:     true,
					},
				},
			},
			"mem": {
				Path:   "/debug/pprof/heap",
				Params: nil, // url.Values{"gc": []string{"1"}},
				SampleTypes: map[string]*SampleTypeConfig{
					"inuse_objects": {
						Units:       "objects",
						Aggregation: "avg",
					},
					"alloc_objects": {
						Units:      "objects",
						Cumulative: true,
					},
					"inuse_space": {
						Units:       "bytes",
						Aggregation: "avg",
					},
					"alloc_space": {
						Units:      "bytes",
						Cumulative: true,
					},
				},
			},
		},

		HTTPClientConfig: DefaultHTTPClientConfig,
		Scheme:           "http",
	}
}

type Config struct {
	// The job name to which the job label is set by default.
	JobName string `yaml:"job-name"`
	// How frequently to scrape the targets of this scrape config.
	ScrapeInterval time.Duration `yaml:"scrape-interval,omitempty"`
	// The timeout for scraping targets of this config.
	ScrapeTimeout time.Duration `yaml:"scrape-timeout,omitempty"`

	// The URL scheme with which to fetch metrics from targets.
	Scheme string `yaml:"scheme,omitempty"`
	// An uncompressed response body larger than this many bytes will cause the
	// scrape to fail. 0 means no limit.
	BodySizeLimit bytesize.ByteSize `yaml:"body-size-limit,omitempty"`
	// TODO(kolesnikovae): Label limits.

	// We cannot do proper Go type embedding below as the parser will then parse
	// values arbitrarily into the overflow maps of further-down types.
	ServiceDiscoveryConfigs discovery.Configs `yaml:"-"`
	HTTPClientConfig        HTTPClientConfig  `yaml:",inline"`

	// List of target relabel configurations.
	RelabelConfigs []*relabel.Config `yaml:"relabel-configs,omitempty"`

	// List of profiles to be scraped.
	EnabledProfiles []string `yaml:"enabled-profiles,omitempty"`
	// Profiles parameters.
	Profiles map[string]*Profile `yaml:"profiles,omitempty"`

	// TODO(kolesnikovae): Implement.
	// List of profiles relabel configurations.
	// ProfilesRelabelConfigs []*relabel.Config `yaml:"profiles-relabel-configs,omitempty"`
}

type Profile struct {
	Path string `yaml:"path,omitempty"`
	// A set of query parameters with which the target is scraped.
	Params url.Values `yaml:"params,omitempty"`
	// SampleTypes contains overrides for pprof sample types.
	SampleTypes map[string]*SampleTypeConfig `yaml:"sample-types,omitempty"`
	// AllSampleTypes specifies whether to parse samples of
	// types not listed in SampleTypes member.
	AllSampleTypes bool `yaml:"all-sample-types,omitempty"`
	// TODO(kolesnikovae): Overrides for interval, timeout, and limits?
}

type SampleTypeConfig struct {
	Units       string `yaml:"units,omitempty"`
	DisplayName string `yaml:"display-name,omitempty"`

	// TODO(kolesnikovae): Introduce Kind?
	//  In Go, we have at least the following combinations:
	//  instant:    Aggregation:avg && !Cumulative && !Sampled
	//  cumulative: Aggregation:sum && Cumulative  && !Sampled
	//  delta:      Aggregation:sum && !Cumulative && Sampled
	Aggregation string `yaml:"aggregation,omitempty"`
	Cumulative  bool   `yaml:"cumulative,omitempty"`
	Sampled     bool   `yaml:"sampled,omitempty"`
}

// SetDirectory joins any relative file paths with dir.
func (c *Config) SetDirectory(dir string) {
	c.ServiceDiscoveryConfigs.SetDirectory(dir)
	c.HTTPClientConfig.SetDirectory(dir)
}

// IsProfileEnabled reports whether the given profile is enabled.
func (c *Config) IsProfileEnabled(p string) bool {
	for _, v := range c.EnabledProfiles {
		if v == p {
			return true
		}
	}
	return false
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	if err := discovery.UnmarshalYAMLWithInlineConfigs(c, unmarshal); err != nil {
		return err
	}
	if len(c.JobName) == 0 {
		return errors.New("job-name is empty")
	}
	if err := mergo.Merge(c, DefaultConfig()); err != nil {
		return fmt.Errorf("failed to apply defaults: %w", err)
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
				if err := CheckTargetAddress(string(t["__name__"])); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func CheckTargetAddress(address string) error {
	if strings.Contains(address, "/") {
		return fmt.Errorf("%q is not a valid hostname", address)
	}
	return nil
}

// MarshalYAML implements the yaml.Marshaler interface.
func (c *Config) MarshalYAML() (interface{}, error) {
	return discovery.MarshalYAMLWithInlineConfigs(c)
}
