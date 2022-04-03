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
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/scrape/config"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/discovery/targetgroup"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

// Manager maintains a set of scrape pools and manages start/stop cycles
// when receiving new target groups from the discovery manager.
type Manager struct {
	logger   logrus.FieldLogger
	ingester Ingester
	stop     chan struct{}

	*metrics
	jitterSeed uint64     // Global jitterSeed seed is used to spread scrape workload across HA setup.
	mtxScrape  sync.Mutex // Guards the fields below.

	scrapeConfigs map[string]*config.Config
	scrapePools   map[string]*scrapePool
	targetSets    map[string][]*targetgroup.Group

	reloadC chan struct{}
}

type Ingester interface {
	Enqueue(context.Context, *storage.PutInput)
}

// NewManager is the Manager constructor
func NewManager(logger logrus.FieldLogger, ingester Ingester, r prometheus.Registerer) *Manager {
	c := make(map[string]*config.Config)
	return &Manager{
		ingester:      ingester,
		logger:        logger,
		scrapeConfigs: c,
		scrapePools:   make(map[string]*scrapePool),
		stop:          make(chan struct{}),
		reloadC:       make(chan struct{}, 1),
		metrics:       newMetrics(r),
	}
}

// Run receives and saves target set updates and triggers the scraping loops reloading.
// Reloading happens in the background so that it doesn't block receiving targets updates.
func (m *Manager) Run(tsets <-chan map[string][]*targetgroup.Group) error {
	m.reload()
	for {
		select {
		case ts := <-tsets:
			m.mtxScrape.Lock()
			m.targetSets = ts
			m.mtxScrape.Unlock()
			m.reload()
		case <-m.stop:
			return nil
		}
	}
}

func (m *Manager) reload() {
	m.mtxScrape.Lock()
	var wg sync.WaitGroup
	for setName, groups := range m.targetSets {
		if _, ok := m.scrapePools[setName]; !ok {
			scrapeConfig, ok := m.scrapeConfigs[setName]
			if !ok {
				m.logger.WithError(fmt.Errorf("invalid config id: %s", setName)).
					WithField("scrape_pool", setName).
					Errorf("reloading target set")
				continue
			}
			sp, err := newScrapePool(scrapeConfig, m.ingester, m.logger, m.metrics)
			if err != nil {
				m.logger.WithError(err).
					WithField("scrape_pool", setName).
					Errorf("creating new scrape pool")
				continue
			}
			m.scrapePools[setName] = sp
		}

		wg.Add(1)
		// Run the sync in parallel as these take a while and at high load can't catch up.
		go func(sp *scrapePool, groups []*targetgroup.Group) {
			sp.Sync(groups)
			wg.Done()
		}(m.scrapePools[setName], groups)
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
	close(m.stop)
}

// ApplyConfig resets the manager's target providers and job configurations as defined by the new cfg.
func (m *Manager) ApplyConfig(cfg []*config.Config) error {
	m.mtxScrape.Lock()
	defer m.mtxScrape.Unlock()
	c := make(map[string]*config.Config)
	for _, x := range cfg {
		c[x.JobName] = x
	}
	m.scrapeConfigs = c
	// Cleanup and reload pool if the configuration has changed.
	var failed bool
	for name, sp := range m.scrapePools {
		cfg, ok := m.scrapeConfigs[name]
		if !ok {
			sp.stop()
			delete(m.scrapePools, name)
			continue
		}
		if reflect.DeepEqual(sp.config, cfg) {
			continue
		}
		if err := sp.reload(cfg); err != nil {
			m.logger.WithError(err).Errorf("reloading scrape pool")
			failed = true
		}
	}

	if failed {
		return errors.New("failed to apply the new configuration")
	}
	return nil
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

	var (
		wg  sync.WaitGroup
		mtx sync.Mutex
	)

	targets := make(map[string][]*Target, len(m.scrapePools))
	wg.Add(len(m.scrapePools))
	for tset, sp := range m.scrapePools {
		// Running in parallel limits the blocking time of scrapePool to scrape
		// interval when there's an update from SD.
		go func(tset string, sp *scrapePool) {
			mtx.Lock()
			targets[tset] = sp.ActiveTargets()
			mtx.Unlock()
			wg.Done()
		}(tset, sp)
	}
	wg.Wait()
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
