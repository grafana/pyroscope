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

// defaultConfig returns the default scrape configuration.
func defaultConfig() *Config {
	return &Config{
		JobName:        "",
		ScrapeInterval: 10 * time.Second,
		ScrapeTimeout:  15 * time.Second,

		EnabledProfiles: nil,
		ProfilingConfigs: ProfilingConfigs{
			ProfileCPU: {
				Path:   "/debug/pprof/profile",
				Params: nil,
			},
			ProfileHeap: {
				Path:   "/debug/pprof/heap",
				Params: nil,
			},
		},

		HTTPClientConfig: DefaultHTTPClientConfig,
		Scheme:           "http",
		BodySizeLimit:    0,

		ServiceDiscoveryConfigs: nil,
		RelabelConfigs:          nil,
	}
}

type Config struct {
	// The job name to which the job label is set by default.
	JobName string `yaml:"job_name"`

	EnabledProfiles  []ProfileName    `yaml:"enabled_profiles,omitempty"`
	ProfilingConfigs ProfilingConfigs `yaml:"profiling_configs,omitempty"`

	// How frequently to scrape the targets of this scrape config.
	ScrapeInterval time.Duration `yaml:"scrape_interval,omitempty"`
	// The timeout for scraping targets of this config.
	ScrapeTimeout time.Duration `yaml:"scrape_timeout,omitempty"`

	// The URL scheme with which to fetch metrics from targets.
	Scheme string `yaml:"scheme,omitempty"`
	// An uncompressed response body larger than this many bytes will cause the
	// scrape to fail. 0 means no limit.
	BodySizeLimit bytesize.ByteSize `yaml:"body_size_limit,omitempty"`

	// We cannot do proper Go type embedding below as the parser will then parse
	// values arbitrarily into the overflow maps of further-down types.

	ServiceDiscoveryConfigs discovery.Configs `yaml:"-"`
	HTTPClientConfig        HTTPClientConfig  `yaml:",inline"`

	// List of target relabel configurations.
	RelabelConfigs []*relabel.Config `yaml:"relabel_configs,omitempty"`

	// TODO(kolesnikovae): Implement.
	// List of profile relabel configurations.
	// ProfileRelabelConfigs []*relabel.Config `yaml:"profile_relabel_configs,omitempty"`
}

type ProfilingConfigs map[ProfileName]ProfilingConfig

type ProfilingConfig struct {
	Path string `yaml:"path,omitempty"`
	// A set of query parameters with which the target is scraped.
	Params url.Values `yaml:"params,omitempty"`
}

// ProfileName designates profiles provided by the runtime/pprof package.
// https://golang.org/doc/diagnostics#profiling
type ProfileName string

const (
	// ProfileCPU determines where a program spends its time while actively
	// consuming CPU cycles (as opposed to while sleeping or waiting for I/O).
	ProfileCPU ProfileName = "cpu"
	// ProfileHeap reports memory allocation samples; used to monitor current
	// and historical memory usage, and to check for memory leaks.
	ProfileHeap ProfileName = "heap"

	// Unsupported yet profile types.

	// ProfileThreadCreate reports the sections of the program that lead
	// the creation of new OS threads.
	ProfileThreadCreate ProfileName = "threadcreate"
	// ProfileGoroutine reports the stack traces of all current goroutines.
	ProfileGoroutine ProfileName = "goroutine"
	// ProfileBlock profile shows where goroutines block waiting on
	// synchronization primitives (including timer channels). Block profile
	// is not enabled by default; use runtime.SetBlockProfileRate to enable it.
	ProfileBlock ProfileName = "block"
	// ProfileMutex profile reports the lock contentions. When you think your
	// CPU is not fully utilized due to a mutex contention, use this profile.
	// Mutex profile is not enabled by default,
	// see runtime.SetMutexProfileFraction to enable it.
	ProfileMutex ProfileName = "mutex"
)

func isSupportedProfileName(p ProfileName) bool {
	switch p {
	case ProfileCPU, ProfileHeap:
		return true
	default:
		return false
	}
}

// SetDirectory joins any relative file paths with dir.
func (c *Config) SetDirectory(dir string) {
	c.ServiceDiscoveryConfigs.SetDirectory(dir)
	c.HTTPClientConfig.SetDirectory(dir)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	if err := discovery.UnmarshalYAMLWithInlineConfigs(c, unmarshal); err != nil {
		return err
	}
	if len(c.JobName) == 0 {
		return errors.New("job_name is empty")
	}
	if err := mergo.Merge(c, defaultConfig()); err != nil {
		return fmt.Errorf("failed to apply defaults: %w", err)
	}

	for x := range c.ProfilingConfigs {
		if !isSupportedProfileName(x) {
			return fmt.Errorf("unsupported profile %q", x)
		}
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
