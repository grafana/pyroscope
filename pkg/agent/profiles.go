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

	"github.com/go-kit/log/level"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"

	phlaremodel "github.com/grafana/pyroscope/pkg/model"

	agentv1v1 "github.com/grafana/pyroscope/api/gen/proto/go/agent/v1"
	"github.com/grafana/pyroscope/pkg/agent/scrape"
)

const (
	pprofMemory     string = "memory"
	pprofBlock      string = "block"
	pprofGoroutine  string = "goroutine"
	pprofMutex      string = "mutex"
	pprofProcessCPU string = "process_cpu"
)

// LabelsByProfiles returns the labels for a given ProfilingConfig.
func LabelsByProfiles(lset labels.Labels, c *scrape.ProfilingConfig) []labels.Labels {
	res := []labels.Labels{}
	add := func(profileType string, cfgs ...scrape.PprofProfilingConfig) {
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
		{Name: model.ScrapeIntervalLabel, Value: cfg.ScrapeInterval.String()},
		{Name: model.ScrapeTimeoutLabel, Value: cfg.ScrapeTimeout.String()},
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
	lset, keep := relabel.Process(preRelabelLabels, cfg.RelabelConfigs...)

	// Check if the target was dropped.
	if !keep {
		return nil, preRelabelLabels, nil
	}
	if v := lset.Get(model.AddressLabel); v == "" {
		return nil, nil, errors.New("no address")
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

	if err := scrape.CheckTargetAddress(model.LabelValue(addr)); err != nil {
		return nil, nil, err
	}

	interval := lset.Get(model.ScrapeIntervalLabel)
	intervalDuration, err := model.ParseDuration(interval)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing scrape interval: %v", err)
	}
	if time.Duration(intervalDuration) == 0 {
		return nil, nil, errors.New("scrape interval cannot be 0")
	}

	timeout := lset.Get(model.ScrapeTimeoutLabel)
	timeoutDuration, err := model.ParseDuration(timeout)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing scrape timeout: %v", err)
	}
	if time.Duration(timeoutDuration) == 0 {
		return nil, nil, errors.New("scrape timeout cannot be 0")
	}

	if timeoutDuration <= intervalDuration {
		return nil, nil, fmt.Errorf("scrape timeout must be larger than scrape interval for (%q <= %q)", timeout, interval)
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

	if serviceName := lset.Get(phlaremodel.LabelNameServiceName); serviceName == "" {
		lb.Set(phlaremodel.LabelNameServiceName, inferServiceName(lset))
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

// intervalAndTimeoutFromLabels returns the interval and timeout derived from the targets labels.
func intervalAndTimeoutFromLabels(
	labels labels.Labels,
	defaultInterval, defaultDuration time.Duration,
) (time.Duration, time.Duration, error) {
	intervalLabel := labels.Get(model.ScrapeIntervalLabel)
	if intervalLabel == "" {
		intervalLabel = defaultInterval.String()
	}
	interval, err := model.ParseDuration(intervalLabel)
	if err != nil {
		return defaultInterval, defaultDuration, fmt.Errorf("error parsing interval label %q: %v", intervalLabel, err)
	}
	timeoutLabel := labels.Get(model.ScrapeTimeoutLabel)
	if timeoutLabel == "" {
		timeoutLabel = defaultDuration.String()
	}
	timeout, err := model.ParseDuration(timeoutLabel)
	if err != nil {
		return defaultInterval, defaultDuration, fmt.Errorf("error parsing timeout label %q: %v", timeoutLabel, err)
	}

	return time.Duration(interval), time.Duration(timeout), nil
}

// TargetsFromGroup builds targets based on the given TargetGroup and config.
func (tg *TargetGroup) TargetsFromGroup(group *targetgroup.Group) ([]*Target, []*Target, error) {
	var (
		targets        = make([]*Target, 0, len(group.Targets))
		droppedTargets = make([]*Target, 0, len(group.Targets))
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

			// The scrape interval and timeout labels are set to the config's values initially,
			// so whether changed via relabeling or not, they'll exist and hold the correct values
			// for every target.
			interval, timeout, err := intervalAndTimeoutFromLabels(
				lbls,
				time.Duration(tg.config.ScrapeInterval),
				time.Duration(tg.config.ScrapeTimeout),
			)
			if err != nil {
				level.Warn(tg.logger).Log(
					"msg", "parse interval and timeout from target labels",
					"job", tg.jobName,
					"err", err,
				)
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
					health:               agentv1v1.Health_HEALTH_UNSPECIFIED,
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
					params.Add("seconds", strconv.Itoa(int(interval/time.Second)-1))
				}
				targets = append(targets, &Target{
					Target:               scrape.NewTarget(lbls, origLabels, params),
					labels:               lbls,
					tenantID:             tg.tenantID,
					scrapeClient:         tg.scrapeClient,
					pusherClientProvider: tg.pusherClientProvider,
					interval:             interval,
					timeout:              timeout,
					health:               agentv1v1.Health_HEALTH_UNSPECIFIED,
					logger:               tg.logger,
				})
			}
		}
	}

	return targets, droppedTargets, nil
}

func inferServiceName(lset labels.Labels) string {
	k8sServiceName := lset.Get(phlaremodel.LabelNameServiceNameK8s)
	if k8sServiceName != "" {
		return k8sServiceName
	}
	k8sNamespace := lset.Get("__meta_kubernetes_namespace")
	k8sContainer := lset.Get("__meta_kubernetes_pod_container_name")
	if k8sNamespace != "" && k8sContainer != "" {
		return fmt.Sprintf("%s/%s", k8sNamespace, k8sContainer)
	}
	dockerContainer := lset.Get("__meta_docker_container_name")
	if dockerContainer != "" {
		return dockerContainer
	}
	return "unspecified"
}
