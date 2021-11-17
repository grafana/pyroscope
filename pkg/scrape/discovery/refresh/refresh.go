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

package refresh

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/scrape/discovery/targetgroup"
)

var (
	failuresCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pyroscope_sd_refresh_failures_total",
			Help: "Number of refresh failures for the given SD mechanism.",
		},
		[]string{"mechanism"},
	)
	duration = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "pyroscope_sd_refresh_duration_seconds",
			Help:       "The duration of a refresh in seconds for the given SD mechanism.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{"mechanism"},
	)
)

func init() {
	prometheus.MustRegister(duration, failuresCount)
}

// Discovery implements the Discoverer interface.
type Discovery struct {
	logger   logrus.FieldLogger
	interval time.Duration
	refreshf func(ctx context.Context) ([]*targetgroup.Group, error)

	failures prometheus.Counter
	duration prometheus.Observer
}

// NewDiscovery returns a Discoverer function that calls a refresh() function at every interval.
func NewDiscovery(l logrus.FieldLogger, mech string, interval time.Duration, refreshf func(ctx context.Context) ([]*targetgroup.Group, error)) *Discovery {
	return &Discovery{
		logger:   l,
		interval: interval,
		refreshf: refreshf,
		failures: failuresCount.WithLabelValues(mech),
		duration: duration.WithLabelValues(mech),
	}
}

// Run implements the Discoverer interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	// Get an initial set right away.
	tgs, err := d.refresh(ctx)
	if err != nil {
		if ctx.Err() != context.Canceled {
			d.logger.WithError(err).Errorf("unable to refresh target groups")
		}
	} else {
		select {
		case ch <- tgs:
		case <-ctx.Done():
			return
		}
	}

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tgs, err = d.refresh(ctx)
			if err != nil {
				if ctx.Err() != context.Canceled {
					d.logger.WithError(err).Errorf("unable to refresh target groups")
				}
				continue
			}

			select {
			case ch <- tgs:
			case <-ctx.Done():
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	now := time.Now()
	defer d.duration.Observe(time.Since(now).Seconds())
	tgs, err := d.refreshf(ctx)
	if err != nil {
		d.failures.Inc()
	}
	return tgs, err
}
