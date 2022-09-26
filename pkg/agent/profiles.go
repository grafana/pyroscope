// Copyright 2021 The Parca Authors
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

// This file is taken from Parca but adapted with our configuration struct.
// We might want to simply use the same configuration struct in the future
// or make Parca easier to reuse.

package agent

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/parca-dev/parca/pkg/config"
	"github.com/parca-dev/parca/pkg/scrape"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"

	agentv1 "github.com/grafana/fire/pkg/gen/agent/v1"
)

const (
	pprofProcessCPU string = "process_cpu"
)

// LabelsByProfiles returns the labels for a given ProfilingConfig.
func LabelsByProfiles(lset labels.Labels, c *config.ProfilingConfig) []labels.Labels {
	res := []labels.Labels{}
	add := func(profileType string, cfgs ...config.PprofProfilingConfig) {
		for _, p := range cfgs {
			if *p.Enabled {
				l := lset.Copy()
				l = append(l, labels.Label{Name: scrape.ProfilePath, Value: p.Path}, labels.Label{Name: scrape.ProfileName, Value: profileType})
				res = append(res, l)
			}
		}
	}

	if c.PprofConfig != nil {
		for profilingType, profilingConfig := range c.PprofConfig {
			add(profilingType, *profilingConfig)
		}
	}

	return res
}

// populateLabels builds a label set from the given label set and scrape configuration.
// It returns a label set before relabeling was applied as the second return value.
// Returns the original discovered label set found before relabelling was applied if the target is dropped during relabeling.
func populateLabels(lset labels.Labels, cfg ScrapeConfig) (res, orig labels.Labels, err error) {
	// Copy labels into the labelset for the target if they are not set already.
	scrapeLabels := []labels.Label{
		{Name: model.JobLabel, Value: cfg.JobName},
		{Name: model.SchemeLabel, Value: cfg.Scheme},
	}
	lb := labels.NewBuilder(lset)

	for _, l := range scrapeLabels {
		if lv := lset.Get(l.Name); lv == "" {
			lb.Set(l.Name, l.Value)
		}
	}
	// Encode scrape query parameters as labels.
	for k, v := range cfg.Params {
		if len(v) > 0 {
			lb.Set(model.ParamLabelPrefix+k, v[0])
		}
	}

	preRelabelLabels := lb.Labels()
	lset = relabel.Process(preRelabelLabels, cfg.RelabelConfigs...)

	// Check if the target was dropped.
	if lset == nil {
		return nil, preRelabelLabels, nil
	}
	if v := lset.Get(model.AddressLabel); v == "" {
		return nil, nil, errors.New("no address")
	}

	if v := lset.Get(model.AddressLabel); v == "" {
		return nil, nil, fmt.Errorf("no address")
	}

	lb = labels.NewBuilder(lset)

	// addPort checks whether we should add a default port to the address.
	// If the address is not valid, we don't append a port either.
	addPort := func(s string) bool {
		// If we can split, a port exists and we don't have to add one.
		if _, _, err := net.SplitHostPort(s); err == nil {
			return false
		}
		// If adding a port makes it valid, the previous error
		// was not due to an invalid address and we can append a port.
		_, _, err := net.SplitHostPort(s + ":1234")
		return err == nil
	}
	addr := lset.Get(model.AddressLabel)
	// If it's an address with no trailing port, infer it based on the used scheme.
	if addPort(addr) {
		// Addresses reaching this point are already wrapped in [] if necessary.
		switch lset.Get(model.SchemeLabel) {
		case "http", "":
			addr = addr + ":80"
		case "https":
			addr = addr + ":443"
		default:
			return nil, nil, fmt.Errorf("invalid scheme: %q", cfg.Scheme)
		}
		lb.Set(model.AddressLabel, addr)
	}

	if err := config.CheckTargetAddress(model.LabelValue(addr)); err != nil {
		return nil, nil, err
	}

	// Meta labels are deleted after relabelling. Other internal labels propagate to
	// the target which decides whether they will be part of their label set.
	for _, l := range lset {
		if strings.HasPrefix(l.Name, model.MetaLabelPrefix) {
			lb.Del(l.Name)
		}
	}

	// Default the instance label to the target address.
	if v := lset.Get(model.InstanceLabel); v == "" {
		lb.Set(model.InstanceLabel, addr)
	}

	res = lb.Labels()
	for _, l := range res {
		// Check label values are valid, drop the target if not.
		if !model.LabelValue(l.Value).IsValid() {
			return nil, nil, fmt.Errorf("invalid label value for %q: %q", l.Name, l.Value)
		}
	}

	return res, lset, nil
}

// targetsFromGroup builds targets based on the given TargetGroup and config.
func (tg *TargetGroup) targetsFromGroup(group *targetgroup.Group) ([]*Target, []*Target, error) {
	var (
		targets        = make([]*Target, 0, len(group.Targets))
		droppedTargets = make([]*Target, 0, len(group.Targets))
		interval       = time.Duration(tg.config.ScrapeInterval)
		timeout        = time.Duration(tg.config.ScrapeTimeout)
	)

	for i, tlset := range group.Targets {
		lbls := make([]labels.Label, 0, len(tlset)+len(group.Labels))

		for ln, lv := range tlset {
			lbls = append(lbls, labels.Label{Name: string(ln), Value: string(lv)})
		}
		for ln, lv := range group.Labels {
			if _, ok := tlset[ln]; !ok {
				lbls = append(lbls, labels.Label{Name: string(ln), Value: string(lv)})
			}
		}

		lset := labels.New(lbls...)
		lsets := scrape.LabelsByProfiles(lset, tg.config.ProfilingConfig)

		for _, lset := range lsets {
			var profType string
			for _, label := range lset {
				if label.Name == scrape.ProfileName {
					profType = label.Value
				}
			}
			lbls, origLabels, err := populateLabels(lset, tg.config)
			if err != nil {
				return nil, nil, fmt.Errorf("instance %d in group %s: %s", i, group, err)
			}
			// This is a dropped target, according to the current return behaviour of populateLabels
			if lbls == nil && origLabels != nil {
				// ensure we get the full url path for dropped targets
				params := tg.config.Params
				if params == nil {
					params = url.Values{}
				}
				lbls = append(lbls, labels.Label{Name: model.AddressLabel, Value: lset.Get(model.AddressLabel)})
				lbls = append(lbls, labels.Label{Name: model.SchemeLabel, Value: tg.config.Scheme})
				lbls = append(lbls, labels.Label{Name: scrape.ProfilePath, Value: lset.Get(scrape.ProfilePath)})
				// Encode scrape query parameters as labels.
				for k, v := range tg.config.Params {
					if len(v) > 0 {
						lbls = append(lbls, labels.Label{Name: model.ParamLabelPrefix + k, Value: v[0]})
					}
				}
				droppedTargets = append(droppedTargets, &Target{
					Target:               scrape.NewTarget(lbls, origLabels, params),
					tenantID:             tg.tenantID,
					labels:               lbls,
					scrapeClient:         tg.scrapeClient,
					pusherClientProvider: tg.pusherClientProvider,
					interval:             interval,
					timeout:              timeout,
					health:               agentv1.Health_HEALTH_UNSPECIFIED,
					logger:               tg.logger,
				})
				continue
			}
			if lbls != nil || origLabels != nil {
				params := tg.config.Params
				if params == nil {
					params = url.Values{}
				}

				if pcfg, found := tg.config.ProfilingConfig.PprofConfig[profType]; found && pcfg.Delta {
					params.Add("seconds", strconv.Itoa(int(time.Duration(tg.config.ScrapeTimeout)/time.Second)-1))
				}
				targets = append(targets, &Target{
					Target:               scrape.NewTarget(lbls, origLabels, params),
					labels:               lbls,
					tenantID:             tg.tenantID,
					scrapeClient:         tg.scrapeClient,
					pusherClientProvider: tg.pusherClientProvider,
					interval:             interval,
					timeout:              timeout,
					health:               agentv1.Health_HEALTH_UNSPECIFIED,
					logger:               tg.logger,
				})
			}
		}
	}

	return targets, droppedTargets, nil
}
