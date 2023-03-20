package agent

import (
	"bytes"
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/parca-dev/parca/pkg/scrape"
	commonconfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/util/pool"
	"golang.org/x/net/context/ctxhttp"

	agentv1 "github.com/grafana/phlare/api/gen/proto/go/agent/v1"
	pushv1 "github.com/grafana/phlare/api/gen/proto/go/push/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	"github.com/grafana/phlare/pkg/tenant"
)

var (
	payloadBuffers  = pool.New(1e3, 1e6, 3, func(sz int) interface{} { return make([]byte, 0, sz) })
	userAgentHeader = fmt.Sprintf("phlare/%s", version.Version)
)

type TargetGroup struct {
	jobName  string
	config   ScrapeConfig
	tenantID string

	logger               log.Logger
	scrapeClient         *http.Client
	pusherClientProvider PusherClientProvider
	ctx                  context.Context

	mtx            sync.RWMutex
	activeTargets  map[uint64]*Target
	droppedTargets []*Target
}

func NewTargetGroup(ctx context.Context, jobName string, cfg ScrapeConfig, pusherClientProvider PusherClientProvider, tenantID string, logger log.Logger) *TargetGroup {
	scrapeClient, err := commonconfig.NewClientFromConfig(cfg.HTTPClientConfig, cfg.JobName)
	if err != nil {
		level.Error(logger).Log("msg", "Error creating HTTP client", "err", err)
	}

	return &TargetGroup{
		jobName:              jobName,
		config:               cfg,
		logger:               logger,
		scrapeClient:         scrapeClient,
		pusherClientProvider: pusherClientProvider,
		ctx:                  ctx,
		activeTargets:        map[uint64]*Target{},
		tenantID:             tenantID,
	}
}

func (tg *TargetGroup) sync(groups []*targetgroup.Group) {
	tg.mtx.Lock()
	defer tg.mtx.Unlock()

	level.Info(tg.logger).Log("msg", "syncing target groups", "job", tg.jobName)
	var actives []*Target
	tg.droppedTargets = []*Target{}
	for _, group := range groups {
		targets, dropped, err := tg.TargetsFromGroup(group)
		if err != nil {
			level.Error(tg.logger).Log("msg", "creating targets failed", "err", err)
			continue
		}
		for _, t := range targets {
			if t.Labels().Len() > 0 {
				actives = append(actives, t)
			}
		}
		tg.droppedTargets = append(tg.droppedTargets, dropped...)
	}

	for _, t := range actives {
		if _, ok := tg.activeTargets[t.Hash()]; !ok {
			tg.activeTargets[t.Hash()] = t
			t.start(tg.ctx)
		} else {
			tg.activeTargets[t.Hash()].SetDiscoveredLabels(t.DiscoveredLabels())
		}
	}

	// Removes inactive targets.
Outer:
	for h, t := range tg.activeTargets {
		for _, at := range actives {
			if h == at.Hash() {
				continue Outer
			}
		}
		t.stop()
		delete(tg.activeTargets, h)
	}
}

type Target struct {
	*scrape.Target
	labels             labels.Labels
	tenantID           string
	mtx                sync.RWMutex
	lastError          error
	lastScrape         time.Time
	lastScrapeDuration time.Duration
	health             agentv1.Health
	lastScrapeSize     int

	scrapeClient         *http.Client
	pusherClientProvider PusherClientProvider

	hash              uint64
	req               *http.Request
	logger            log.Logger
	interval, timeout time.Duration
	cancel            context.CancelFunc
}

func (t *Target) start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	t.cancel = cancel
	go func() {
		defer cancel()
		select {
		case <-time.After(t.offset()):
		case <-ctx.Done():
			return
		}
		ticker := time.NewTicker(t.interval)
		defer ticker.Stop()

		tick := func() {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
		for ; true; tick() {
			if ctx.Err() != nil {
				return
			}
			t.scrape(ctx)
		}
	}()
}

func (t *Target) scrape(ctx context.Context) {
	var (
		start             = time.Now()
		b                 = payloadBuffers.Get(t.lastScrapeSize).([]byte)
		buf               = bytes.NewBuffer(b)
		profileType       string
		scrapeCtx, cancel = context.WithTimeout(ctx, t.timeout)
	)
	defer cancel()

	for _, l := range t.labels {
		if l.Name == scrape.ProfileName {
			profileType = l.Value
			break
		}
	}

	if err := t.fetchProfile(scrapeCtx, profileType, buf); err != nil {
		level.Error(t.logger).Log("msg", "fetch profile failed", "target", t.Labels().String(), "err", err)
		t.health = agentv1.Health_HEALTH_DOWN
		t.lastScrapeDuration = time.Since(start)
		t.lastError = err
		t.lastScrape = start
		return
	}

	b = buf.Bytes()
	if len(b) > 0 {
		t.lastScrapeSize = len(b)
	}
	t.health = agentv1.Health_HEALTH_UP
	t.lastScrapeDuration = time.Since(start)
	t.lastError = nil
	t.lastScrape = start
	// todo retry strategy
	req := &pushv1.PushRequest{}
	series := &pushv1.RawProfileSeries{
		Labels: make([]*typesv1.LabelPair, 0, len(t.labels)),
	}
	for _, l := range t.labels {
		if strings.HasPrefix(l.Name, "__") && l.Name != labels.MetricName {
			continue
		}
		series.Labels = append(series.Labels, &typesv1.LabelPair{
			Name:  l.Name,
			Value: l.Value,
		})
	}
	series.Samples = []*pushv1.RawSample{
		{
			RawProfile: b,
		},
	}
	req.Series = append(req.Series, series)
	// Inject the tenant ID into the context.
	// With a http pusher the interceptor will add the tenant ID to the request headers.
	// When directly pushing distributors, the tenant ID will already be in the context.
	if t.tenantID != "" {
		ctx = tenant.InjectTenantID(ctx, t.tenantID)
	}
	if _, err := t.pusherClientProvider().Push(ctx, connect.NewRequest(req)); err != nil {
		level.Error(t.logger).Log("msg", "push failed", "labels", t.Labels().String(), "err", err)
	}
}

func (t *Target) fetchProfile(ctx context.Context, profileType string, buf io.Writer) error {
	if t.req == nil {
		req, err := http.NewRequest("GET", t.URL().String(), nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", userAgentHeader)
		req.Header.Set("Accept-Encoding", "gzip")

		t.req = req
	}

	level.Debug(t.logger).Log("msg", "scraping profile", "labels", t.Labels().String(), "url", t.req.URL.String())
	resp, err := ctxhttp.Do(ctx, t.scrapeClient, t.req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(io.TeeReader(resp.Body, buf))
	if err != nil {
		return fmt.Errorf("failed to read body: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		if len(b) > 0 {
			return fmt.Errorf("server returned HTTP status (%d) %v", resp.StatusCode, string(bytes.TrimSpace(b)))
		}
		return fmt.Errorf("server returned HTTP status (%d) %v", resp.StatusCode, resp.Status)
	}

	if len(b) == 0 {
		return fmt.Errorf("empty %s profile from %s", profileType, t.req.URL.String())
	}
	return nil
}

func (t *Target) stop() {
	t.cancel()
}

// hash returns an identifying hash for the target.
func (t *Target) Hash() uint64 {
	if t.hash != 0 {
		return t.hash
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(fmt.Sprintf("%016d", t.labels.Hash())))
	_, _ = h.Write([]byte(t.URL().String()))
	t.hash = h.Sum64()
	return t.hash
}

// offset returns the time until the next scrape cycle for the target.
func (t *Target) offset() time.Duration {
	now := time.Now().UnixNano()

	var (
		base   = now % int64(t.interval)
		offset = t.Hash() % uint64(t.interval)
		next   = base + int64(offset)
	)

	if next > int64(t.interval) {
		next -= int64(t.interval)
	}
	return time.Duration(next)
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
func (t *Target) Health() agentv1.Health {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.health
}

func (t *Target) Labels() labels.Labels {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.labels
}

// GetValue gets a label value from the entire label set.
func (t *Target) GetValue(name string) string {
	return t.labels.Get(name)
}
