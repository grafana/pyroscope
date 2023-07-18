// Copyright 2022-2023 The Parca Authors
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
	"strings"
	"sync"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"

	"github.com/parca-dev/parca/pkg/config"
)

// TargetHealth describes the health state of a target.
type TargetHealth string

// The possible health states of a target based on the last performed scrape.
const (
	HealthUnknown TargetHealth = "unknown"
	HealthGood    TargetHealth = "up"
	HealthBad     TargetHealth = "down"
)

// Target refers to a singular HTTP or HTTPS endpoint.
type Target struct {
	// Labels before any processing.
	discoveredLabels labels.Labels
	// Any labels that are added to this target and its metrics.
	labels labels.Labels
	// Additional URL parmeters that are part of the target URL.
	params url.Values

	mtx                sync.RWMutex
	lastError          error
	lastScrape         time.Time
	lastScrapeDuration time.Duration
	health             TargetHealth
}

// NewTarget creates a reasonably configured target for querying.
func NewTarget(labels, discoveredLabels labels.Labels, params url.Values) *Target {
	return &Target{
		labels:           labels,
		discoveredLabels: discoveredLabels,
		params:           params,
		health:           HealthUnknown,
	}
}

func (t *Target) String() string {
	return t.URL().String()
}

// Params returns a copy of the set of all public params of the target.
func (t *Target) Params() url.Values {
	q := make(url.Values, len(t.params))
	for k, values := range t.params {
		q[k] = make([]string, len(values))
		copy(q[k], values)
	}
	return q
}

// Labels returns a copy of the set of all public labels of the target.
func (t *Target) Labels() labels.Labels {
	b := labels.NewScratchBuilder(t.labels.Len())
	t.labels.Range(func(l labels.Label) {
		if !strings.HasPrefix(l.Name, model.ReservedLabelPrefix) {
			b.Add(l.Name, l.Value)
		}
	})
	return b.Labels()
}

// DiscoveredLabels returns a copy of the target's labels before any processing.
func (t *Target) DiscoveredLabels() labels.Labels {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	return t.discoveredLabels.Copy()
}

// Clone returns a clone of the target.
func (t *Target) Clone() *Target {
	return NewTarget(
		t.Labels(),
		t.DiscoveredLabels(),
		t.Params(),
	)
}

// SetDiscoveredLabels sets new DiscoveredLabels.
func (t *Target) SetDiscoveredLabels(l labels.Labels) {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	t.discoveredLabels = l
}

// URL returns a copy of the target's URL.
func (t *Target) URL() *url.URL {
	params := url.Values{}

	for k, v := range t.params {
		params[k] = make([]string, len(v))
		copy(params[k], v)
	}
	t.labels.Range(func(l labels.Label) {
		if !strings.HasPrefix(l.Name, model.ParamLabelPrefix) {
			return
		}
		ks := l.Name[len(model.ParamLabelPrefix):]

		if len(params[ks]) > 0 {
			params[ks][0] = l.Value
		} else {
			params[ks] = []string{l.Value}
		}
	})

	return &url.URL{
		Scheme:   t.labels.Get(model.SchemeLabel),
		Host:     t.labels.Get(model.AddressLabel),
		Path:     t.labels.Get(ProfilePath),
		RawQuery: params.Encode(),
	}
}

// LastError returns the error encountered during the last scrape.
func (t *Target) LastError() error {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.lastError
}

// LastScrape returns the time of the last scrape.
func (t *Target) LastScrape() time.Time {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.lastScrape
}

// LastScrapeDuration returns how long the last scrape of the target took.
func (t *Target) LastScrapeDuration() time.Duration {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.lastScrapeDuration
}

// Health returns the last known health state of the target.
func (t *Target) Health() TargetHealth {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.health
}

// LabelsByProfiles returns the labels for a given ProfilingConfig.
func LabelsByProfiles(lset labels.Labels, c *config.ProfilingConfig) []labels.Labels {
	res := []labels.Labels{}

	if len(c.PprofConfig) > 0 {
		b := labels.NewScratchBuilder(lset.Len() + 2)
		for profilingType, profilingConfig := range c.PprofConfig {
			if *profilingConfig.Enabled {
				b.Assign(lset)
				b.Add(ProfilePath, profilingConfig.Path)
				b.Add(ProfileName, profilingType)
				res = append(res, b.Labels())
			}
		}
	}

	return res
}

// Targets is a sortable list of targets.
type Targets []*Target

func (ts Targets) Len() int           { return len(ts) }
func (ts Targets) Less(i, j int) bool { return ts[i].URL().String() < ts[j].URL().String() }
func (ts Targets) Swap(i, j int)      { ts[i], ts[j] = ts[j], ts[i] }

const (
	ProfilePath      = "__profile_path__"
	ProfileName      = "__name__"
	ProfileTraceType = "trace"
)
