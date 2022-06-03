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

package scrape

import (
	"reflect"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"

	profilepb "github.com/parca-dev/parca/gen/proto/go/parca/profilestore/v1alpha1"
	scrapepb "github.com/parca-dev/parca/gen/proto/go/parca/scrape/v1alpha1"
	"github.com/parca-dev/parca/pkg/config"
)

// NewManager is the Manager constructor.
func NewManager(
	logger log.Logger,
	reg prometheus.Registerer,
	store profilepb.ProfileStoreServiceServer,
	scrapeConfigs []*config.ScrapeConfig,
	externalLabels labels.Labels,
) *Manager {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	m := &Manager{
		store:         store,
		logger:        logger,
		scrapeConfigs: make(map[string]*config.ScrapeConfig),
		scrapePools:   make(map[string]*scrapePool),
		graceShut:     make(chan struct{}),
		triggerReload: make(chan struct{}, 1),

		externalLabels: externalLabels,

		targetIntervalLength: prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Name:       "parca_target_interval_length_seconds",
				Help:       "Actual intervals between scrapes.",
				Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
			}, []string{"interval"}),
		targetReloadIntervalLength: prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Name:       "parca_target_reload_length_seconds",
				Help:       "Actual interval to reload the scrape pool with a given configuration.",
				Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
			}, []string{"interval"}),
		targetSyncIntervalLength: prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Name:       "parca_target_sync_length_seconds",
				Help:       "Actual interval to sync the scrape pool.",
				Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
			}, []string{"scrape_job"}),
		targetScrapePoolSyncsCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "parca_target_scrape_pool_sync_total",
				Help: "Total number of syncs that were executed on a scrape pool.",
			}, []string{"scrape_job"}),
		targetScrapeSampleLimit: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "parca_target_scrapes_exceeded_sample_limit_total",
				Help: "Total number of scrapes that hit the sample limit and were rejected.",
			}),
		targetScrapeSampleDuplicate: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "parca_target_scrapes_sample_duplicate_timestamp_total",
				Help: "Total number of samples rejected due to duplicate timestamps but different values",
			}),
		targetScrapeSampleOutOfOrder: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "parca_target_scrapes_sample_out_of_order_total",
				Help: "Total number of samples rejected due to not being out of the expected order",
			}),
		targetScrapeSampleOutOfBounds: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "parca_target_scrapes_sample_out_of_bounds_total",
				Help: "Total number of samples rejected due to timestamp falling outside of the time bounds",
			}),
	}

	reg.MustRegister(
		m.targetIntervalLength,
		m.targetReloadIntervalLength,
		m.targetSyncIntervalLength,
		m.targetScrapePoolSyncsCounter,
		m.targetScrapeSampleLimit,
		m.targetScrapeSampleDuplicate,
		m.targetScrapeSampleOutOfOrder,
		m.targetScrapeSampleOutOfBounds,
	)

	c := make(map[string]*config.ScrapeConfig)
	for _, scfg := range scrapeConfigs {
		c[scfg.JobName] = scfg
	}
	m.scrapeConfigs = c

	// Cleanup and reload pool if config has changed.
	for name, sp := range m.scrapePools {
		if cfg, ok := m.scrapeConfigs[name]; !ok {
			sp.stop()
			delete(m.scrapePools, name)
		} else if !reflect.DeepEqual(sp.config, cfg) {
			sp.reload(cfg)
		}
	}

	return m
}

// Manager maintains a set of scrape pools and manages start/stop cycles
// when receiving new target groups form the discovery manager.
type Manager struct {
	scrapepb.UnimplementedScrapeServiceServer

	logger    log.Logger
	store     profilepb.ProfileStoreServiceServer
	graceShut chan struct{}

	externalLabels labels.Labels

	mtxScrape     sync.Mutex // Guards the fields below.
	scrapeConfigs map[string]*config.ScrapeConfig
	scrapePools   map[string]*scrapePool
	targetSets    map[string][]*targetgroup.Group

	triggerReload chan struct{}

	targetIntervalLength          *prometheus.SummaryVec
	targetReloadIntervalLength    *prometheus.SummaryVec
	targetSyncIntervalLength      *prometheus.SummaryVec
	targetScrapePoolSyncsCounter  *prometheus.CounterVec
	targetScrapeSampleLimit       prometheus.Counter
	targetScrapeSampleDuplicate   prometheus.Counter
	targetScrapeSampleOutOfOrder  prometheus.Counter
	targetScrapeSampleOutOfBounds prometheus.Counter
}

// Run stars the manager with a set of scrape configs.
func (m *Manager) Run(tsets <-chan map[string][]*targetgroup.Group) error {
	go m.reloader()
	for {
		select {
		case ts := <-tsets:
			m.updateTsets(ts)

			select {
			case m.triggerReload <- struct{}{}:
			default:
			}

		case <-m.graceShut:
			return nil
		}
	}
}

func (m *Manager) reloader() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.graceShut:
			return
		case <-ticker.C:
			select {
			case <-m.triggerReload:
				m.reload()
			case <-m.graceShut:
				return
			}
		}
	}
}

func (m *Manager) reload() {
	m.mtxScrape.Lock()
	var wg sync.WaitGroup
	level.Debug(m.logger).Log("msg", "Reloading scrape manager")
	for setName, groups := range m.targetSets {
		var sp *scrapePool
		existing, ok := m.scrapePools[setName]
		if !ok {
			scrapeConfig, ok := m.scrapeConfigs[setName]
			if !ok {
				level.Error(m.logger).Log("msg", "error reloading target set", "err", "invalid config id:"+setName)
				return
			}
			sp = newScrapePool(scrapeConfig, m.store, log.With(m.logger, "scrape_pool", setName), m.externalLabels, &scrapePoolMetrics{
				targetIntervalLength:          m.targetIntervalLength,
				targetReloadIntervalLength:    m.targetReloadIntervalLength,
				targetSyncIntervalLength:      m.targetSyncIntervalLength,
				targetScrapePoolSyncsCounter:  m.targetScrapePoolSyncsCounter,
				targetScrapeSampleLimit:       m.targetScrapeSampleLimit,
				targetScrapeSampleDuplicate:   m.targetScrapeSampleDuplicate,
				targetScrapeSampleOutOfOrder:  m.targetScrapeSampleOutOfOrder,
				targetScrapeSampleOutOfBounds: m.targetScrapeSampleOutOfBounds,
			})
			m.scrapePools[setName] = sp
		} else {
			sp = existing
		}

		wg.Add(1)
		// Run the sync in parallel as these take a while and at high load can't catch up.
		go func(sp *scrapePool, groups []*targetgroup.Group) {
			sp.Sync(groups)
			wg.Done()
		}(sp, groups)
	}
	m.mtxScrape.Unlock()
	wg.Wait()
}

// Stop cancels all running scrape pools and blocks until all have exited.
func (m *Manager) Stop() {
	m.mtxScrape.Lock()
	defer m.mtxScrape.Unlock()

	for _, sp := range m.scrapePools {
		sp.stop()
	}
	close(m.graceShut)
}

func (m *Manager) updateTsets(tsets map[string][]*targetgroup.Group) {
	m.mtxScrape.Lock()
	m.targetSets = tsets
	m.mtxScrape.Unlock()
}

// TargetsAll returns active and dropped targets grouped by job_name.
func (m *Manager) TargetsAll() map[string][]*Target {
	m.mtxScrape.Lock()
	defer m.mtxScrape.Unlock()

	targets := make(map[string][]*Target, len(m.scrapePools))
	for tset, sp := range m.scrapePools {
		targets[tset] = append(sp.ActiveTargets(), sp.DroppedTargets()...)
	}
	return targets
}

// TargetsActive returns the active targets currently being scraped.
func (m *Manager) TargetsActive() map[string][]*Target {
	m.mtxScrape.Lock()
	defer m.mtxScrape.Unlock()

	targets := make(map[string][]*Target, len(m.scrapePools))
	for tset, sp := range m.scrapePools {
		targets[tset] = sp.ActiveTargets()
	}
	return targets
}

// TargetsDropped returns the dropped targets during relabelling.
func (m *Manager) TargetsDropped() map[string][]*Target {
	m.mtxScrape.Lock()
	defer m.mtxScrape.Unlock()

	targets := make(map[string][]*Target, len(m.scrapePools))
	for tset, sp := range m.scrapePools {
		targets[tset] = sp.DroppedTargets()
	}
	return targets
}

// ApplyConfig resets the manager's target providers and job configurations as defined by the new cfg.
func (m *Manager) ApplyConfig(cfgs []*config.ScrapeConfig) error {
	m.mtxScrape.Lock()
	defer m.mtxScrape.Unlock()

	c := make(map[string]*config.ScrapeConfig)
	for _, scfg := range cfgs {
		c[scfg.JobName] = scfg
	}
	m.scrapeConfigs = c

	// Cleanup and reload pool if config has changed.
	for name, sp := range m.scrapePools {
		if cfg, ok := m.scrapeConfigs[name]; !ok {
			sp.stop()
			delete(m.scrapePools, name)
		} else if !reflect.DeepEqual(sp.config, cfg) {
			sp.reload(cfg)
		}
	}

	return nil
}
