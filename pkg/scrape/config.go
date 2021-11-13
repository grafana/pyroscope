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

package scrape

import (
	"net/url"
	"time"

	"github.com/prometheus/common/config"

	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

type Config struct {
	ScrapeInterval time.Duration     `yaml:"scrape_interval,omitempty"`
	ScrapeTimeout  time.Duration     `yaml:"scrape_timeout,omitempty"`
	ExternalLabels map[string]string `yaml:"external_labels,omitempty"`

	ScrapeConfigs     []*ScrapeConfig `yaml:"scrape_configs,omitempty"`
	ScrapeConfigFiles []string        `yaml:"scrape_config_files,omitempty"`
}

type ScrapeConfig struct {
	// The job name to which the job label is set by default.
	JobName string `yaml:"job_name"`

	EnabledProfiles  []ProfileName    `yaml:"enabled_profiles,omitempty"`
	ProfilingConfigs ProfilingConfigs `yaml:"profiling_configs,omitempty"`
	StaticConfigs    []StaticConfig   `yaml:"static_configs,omitempty"`

	// How frequently to scrape the targets of this scrape config.
	ScrapeInterval time.Duration `yaml:"scrape_interval,omitempty"`
	// The timeout for scraping targets of this config.
	ScrapeTimeout    time.Duration           `yaml:"scrape_timeout,omitempty"`
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`
	// The URL scheme with which to fetch metrics from targets.
	Scheme string `yaml:"scheme,omitempty"`
	// An uncompressed response body larger than this many bytes will cause the
	// scrape to fail. 0 means no limit.
	BodySizeLimit bytesize.ByteSize `yaml:"body_size_limit,omitempty"`
}

type StaticConfig struct {
	Labels  map[string]string `yaml:"labels,omitempty"`
	Targets []string          `yaml:"targets,omitempty"`
}

type ProfilingConfigs map[ProfileName]ProfilingConfig

type ProfilingConfig struct {
	Path string `yaml:"path,omitempty"`
	// A set of query parameters with which the target is scraped.
	Params url.Values `yaml:"params,omitempty"`
}

// Group is a set of targets with a common label set(production , test, staging etc.).
type Group struct {
	// Targets is a list of targets identified by a label set. Each target is
	// uniquely identifiable in the group by its address label.
	Targets []map[string]string
	// Labels is a set of labels that is common across all targets in the group.
	Labels map[string]string
	// Source is an identifier that describes a group of targets.
	Source string
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
