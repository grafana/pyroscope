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
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/valyala/bytebufferpool"

	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/config"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/discovery/targetgroup"
)

var UserAgent = fmt.Sprintf("Pyroscope/%s", build.Version)

var errBodySizeLimit = errors.New("body size limit exceeded")

// scrapePool manages scrapes for sets of targets.
type scrapePool struct {
	upstream upstream.Upstream
	logger   logrus.FieldLogger

	// Global metrics shared by all pools.
	metrics *metrics
	// Job-specific metrics.
	poolMetrics *poolMetrics

	ctx    context.Context
	cancel context.CancelFunc

	// mtx must not be taken after targetMtx.
	mtx    sync.Mutex
	config *config.Config
	client *http.Client
	loops  map[uint64]*scrapeLoop

	targetMtx sync.Mutex
	// activeTargets and loops must always be synchronized to have the same
	// set of hashes.
	activeTargets  map[uint64]*Target
	droppedTargets []*Target
}

func newScrapePool(cfg *config.Config, u upstream.Upstream, logger logrus.FieldLogger, m *metrics) (*scrapePool, error) {
	m.pools.Inc()

	client, err := config.NewClientFromConfig(cfg.HTTPClientConfig, cfg.JobName)
	if err != nil {
		m.poolsFailed.Inc()
		return nil, fmt.Errorf("creating HTTP client: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	sp := scrapePool{
		ctx:           ctx,
		cancel:        cancel,
		logger:        logger,
		upstream:      u,
		config:        cfg,
		client:        client,
		activeTargets: make(map[uint64]*Target),
		loops:         make(map[uint64]*scrapeLoop),

		metrics:     m,
		poolMetrics: m.poolMetrics(cfg.JobName),
	}

	return &sp, nil
}

func (sp *scrapePool) newScrapeLoop(s *scraper, i, t time.Duration) *scrapeLoop {
	x := scrapeLoop{
		scraper:     s,
		logger:      sp.logger,
		upstream:    sp.upstream,
		poolMetrics: sp.poolMetrics,
		stopped:     make(chan struct{}),
		interval:    i,
		timeout:     t,
	}
	x.ctx, x.cancel = context.WithCancel(sp.ctx)
	return &x
}

func (sp *scrapePool) ActiveTargets() []*Target {
	sp.targetMtx.Lock()
	defer sp.targetMtx.Unlock()
	var tActive []*Target
	for _, t := range sp.activeTargets {
		tActive = append(tActive, t)
	}
	return tActive
}

func (sp *scrapePool) DroppedTargets() []*Target {
	sp.targetMtx.Lock()
	defer sp.targetMtx.Unlock()
	return sp.droppedTargets
}

// stop terminates all scrapers and returns after they all terminated.
func (sp *scrapePool) stop() {
	sp.mtx.Lock()
	defer sp.mtx.Unlock()
	sp.cancel()
	sp.targetMtx.Lock()
	var wg sync.WaitGroup
	wg.Add(len(sp.loops))
	for fp, l := range sp.loops {
		go func(l *scrapeLoop) {
			l.stop()
			wg.Done()
		}(l)
		delete(sp.loops, fp)
		delete(sp.activeTargets, fp)
	}
	sp.targetMtx.Unlock()
	wg.Wait()
	sp.client.CloseIdleConnections()
	if sp.config == nil {
		return
	}
	sp.metrics.scrapeIntervalLength.DeleteLabelValues(sp.config.JobName)
	sp.metrics.poolReloadIntervalLength.DeleteLabelValues(sp.config.JobName)
	sp.metrics.poolSyncIntervalLength.DeleteLabelValues(sp.config.JobName)
	sp.metrics.poolSyncs.DeleteLabelValues(sp.config.JobName)
	sp.metrics.poolSyncFailed.DeleteLabelValues(sp.config.JobName)
	sp.metrics.poolTargetsAdded.DeleteLabelValues(sp.config.JobName)
	sp.metrics.scrapesFailed.DeleteLabelValues(sp.config.JobName)
	sp.metrics.scrapeDuration.DeleteLabelValues(sp.config.JobName)
	sp.metrics.scrapeBodySize.DeleteLabelValues(sp.config.JobName)
}

// reload the scrape pool with the given scrape configuration. The target state is preserved
// but all scrapers are restarted with the new scrape configuration.
func (sp *scrapePool) reload(cfg *config.Config) error {
	sp.mtx.Lock()
	defer sp.mtx.Unlock()
	sp.metrics.poolReloads.Inc()
	start := time.Now()

	client, err := config.NewClientFromConfig(cfg.HTTPClientConfig, cfg.JobName)
	if err != nil {
		sp.metrics.poolReloadsFailed.Inc()
		return fmt.Errorf("creating HTTP client: %w", err)
	}

	sp.config = cfg
	oldClient := sp.client
	sp.client = client

	var (
		wg            sync.WaitGroup
		interval      = sp.config.ScrapeInterval
		timeout       = sp.config.ScrapeTimeout
		bodySizeLimit = int64(sp.config.BodySizeLimit)
	)

	sp.targetMtx.Lock()
	for fp, oldLoop := range sp.loops {
		wg.Add(1)
		s := &scraper{
			Target:        sp.activeTargets[fp],
			pprofWriter:   newPprofWriter(sp.upstream, sp.activeTargets[fp]),
			client:        sp.client,
			timeout:       timeout,
			bodySizeLimit: bodySizeLimit,
		}
		n := sp.newScrapeLoop(s, interval, timeout)
		go func(oldLoop, newLoop *scrapeLoop) {
			oldLoop.stop()
			wg.Done()
			newLoop.run()
		}(oldLoop, n)
		sp.loops[fp] = n
	}

	sp.targetMtx.Unlock()
	wg.Wait()
	oldClient.CloseIdleConnections()
	sp.poolMetrics.poolReloadIntervalLength.Observe(time.Since(start).Seconds())
	return nil
}

// Sync converts target groups into actual scrape targets and synchronizes
// the currently running scraper with the resulting set and returns all scraped and dropped targets.
func (sp *scrapePool) Sync(tgs []*targetgroup.Group) {
	sp.mtx.Lock()
	defer sp.mtx.Unlock()
	start := time.Now()

	sp.targetMtx.Lock()
	var all []*Target
	sp.droppedTargets = []*Target{}
	for _, tg := range tgs {
		targets, failures := TargetsFromGroup(tg, sp.config)
		for _, err := range failures {
			sp.logger.WithError(err).Errorf("creating target")
		}
		sp.poolMetrics.poolSyncFailed.Add(float64(len(failures)))
		for _, t := range targets {
			if t.Labels().Len() > 0 {
				all = append(all, t)
			} else if t.DiscoveredLabels().Len() > 0 {
				sp.droppedTargets = append(sp.droppedTargets, t)
			}
		}
	}
	sp.targetMtx.Unlock()
	sp.sync(all)

	sp.poolMetrics.poolSyncIntervalLength.Observe(time.Since(start).Seconds())
	sp.poolMetrics.poolSyncs.Inc()
}

// revive:disable:confusing-naming private
// revive:disable:import-shadowing methods don't shadow imports
func (sp *scrapePool) sync(targets []*Target) {
	var (
		uniqueLoops   = make(map[uint64]*scrapeLoop)
		interval      = sp.config.ScrapeInterval
		timeout       = sp.config.ScrapeTimeout
		bodySizeLimit = int64(sp.config.BodySizeLimit)
	)

	sp.targetMtx.Lock()
	for _, t := range targets {
		hash := t.hash()
		_, ok := sp.activeTargets[hash]
		if ok {
			if _, ok := uniqueLoops[hash]; !ok {
				uniqueLoops[hash] = nil
			}
			continue
		}

		var err error
		interval, timeout, err = t.intervalAndTimeout(interval, timeout)
		if err != nil {
			sp.logger.WithError(err).Errorf("invalid target label")
		}

		s := &scraper{
			Target:        t,
			client:        sp.client,
			timeout:       timeout,
			bodySizeLimit: bodySizeLimit,
			pprofWriter:   newPprofWriter(sp.upstream, t),
		}

		l := sp.newScrapeLoop(s, interval, timeout)
		sp.activeTargets[hash] = t
		sp.loops[hash] = l
		uniqueLoops[hash] = l
	}

	var wg sync.WaitGroup
	for hash := range sp.activeTargets {
		if _, ok := uniqueLoops[hash]; !ok {
			wg.Add(1)
			go func(l *scrapeLoop) {
				l.stop()
				wg.Done()
			}(sp.loops[hash])
			delete(sp.loops, hash)
			delete(sp.activeTargets, hash)
		}
	}

	sp.targetMtx.Unlock()
	sp.poolMetrics.poolTargetsAdded.Set(float64(len(uniqueLoops)))
	for _, l := range uniqueLoops {
		if l != nil {
			go l.run()
		}
	}

	wg.Wait()
}

type scrapeLoop struct {
	scraper  *scraper
	logger   logrus.FieldLogger
	upstream upstream.Upstream

	poolMetrics *poolMetrics

	ctx     context.Context
	cancel  func()
	stopped chan struct{}

	interval time.Duration
	timeout  time.Duration
}

var bufPool = bytebufferpool.Pool{}

func (sl *scrapeLoop) run() {
	defer close(sl.stopped)
	select {
	case <-time.After(sl.scraper.offset(sl.interval)):
	case <-sl.ctx.Done():
		return
	}
	ticker := time.NewTicker(sl.interval)
	defer ticker.Stop()
	var last time.Time
	for {
		select {
		default:
		case <-sl.ctx.Done():
			return
		}

		if !last.IsZero() {
			sl.poolMetrics.scrapeIntervalLength.Observe(time.Since(last).Seconds())
		}
		last = sl.scraper.report(sl.scrape)
		sl.poolMetrics.scrapeDuration.Observe(sl.scraper.Target.lastScrapeDuration.Seconds())
		select {
		case <-ticker.C:
		case <-sl.ctx.Done():
			return
		}
	}
}

func (sl *scrapeLoop) scrape() error {
	buf := bufPool.Get()
	ctx, cancel := context.WithTimeout(sl.ctx, sl.timeout)
	defer func() {
		bufPool.Put(buf)
		cancel()
	}()
	sl.poolMetrics.scrapes.Inc()
	switch err := sl.scraper.scrape(ctx, buf); {
	case err == nil:
		sl.poolMetrics.scrapeBodySize.Observe(float64(buf.Len()))
		return sl.scraper.pprofWriter.writeProfile(buf.Bytes())
	case errors.Is(err, context.Canceled):
		return nil
	default:
		sl.poolMetrics.scrapesFailed.Inc()
		sl.logger.WithError(err).WithField("target", sl.scraper.Target.String()).Debug("scrapping failed")
		sl.scraper.pprofWriter.reset()
		return err
	}
}

func (sl *scrapeLoop) stop() {
	sl.cancel()
	<-sl.stopped
}

type scraper struct {
	*Target
	*pprofWriter

	client  *http.Client
	req     *http.Request
	timeout time.Duration

	buf           *bufio.Reader
	bodySizeLimit int64
}

func (s *scraper) scrape(ctx context.Context, w io.Writer) error {
	if s.req == nil {
		req, err := http.NewRequest("GET", s.URL().String(), nil)
		if err != nil {
			return err
		}
		req.Header.Add("Accept-Encoding", "gzip")
		req.Header.Set("User-Agent", UserAgent)
		s.req = req
	}

	resp, err := s.client.Do(s.req.WithContext(ctx))
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned HTTP status %s", resp.Status)
	}

	if s.bodySizeLimit <= 0 {
		s.bodySizeLimit = math.MaxInt64
	}

	if s.buf == nil {
		s.buf = bufio.NewReader(resp.Body)
	} else {
		s.buf.Reset(resp.Body)
	}

	header, err := s.buf.Peek(2)
	if err != nil {
		return err
	}

	r := resp.Body
	if header[0] == 0x1f && header[1] == 0x8b {
		gzipr, err := gzip.NewReader(s.buf)
		if err != nil {
			return err
		}
		r = gzipr
		defer gzipr.Close()
	}

	n, err := io.Copy(w, io.LimitReader(r, s.bodySizeLimit))
	if err != nil {
		return err
	}
	if n >= s.bodySizeLimit {
		return errBodySizeLimit
	}
	return nil
}
