package aggregator

import (
	"context"
	"sync"
	"time"

	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"go.uber.org/atomic"
)

type MultiTenantAggregator[T any] struct {
	*services.BasicService

	limits     Limits
	registerer prometheus.Registerer

	m       sync.RWMutex
	tenants map[tenantKey]*tenantAggregator[T]

	closeOnce sync.Once
	stop      chan struct{}
	done      chan struct{}
}

type Limits interface {
	DistributorAggregationWindow(tenantID string) model.Duration
	DistributorAggregationPeriod(tenantID string) model.Duration
}

func NewMultiTenantAggregator[T any](limits Limits, registerer prometheus.Registerer) *MultiTenantAggregator[T] {
	m := MultiTenantAggregator[T]{
		limits:     limits,
		registerer: registerer,
		tenants:    make(map[tenantKey]*tenantAggregator[T]),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}
	m.BasicService = services.NewBasicService(
		m.starting,
		m.running,
		m.stopping,
	)
	return &m
}

type tenantAggregator[T any] struct {
	lastSeen   atomic.Time
	key        tenantKey
	collector  prometheus.Collector
	registerer prometheus.Registerer
	aggregator *Aggregator[T]
}

type tenantKey struct {
	tenantID string
	window   model.Duration
	period   model.Duration
}

func (m *MultiTenantAggregator[T]) AggregatorForTenant(tenantID string) (*Aggregator[T], bool) {
	now := time.Now()
	t, ok := m.aggregatorForTenant(tenantID)
	if ok {
		t.lastSeen.Store(now)
		return t.aggregator, true
	}
	return nil, false
}

func (m *MultiTenantAggregator[T]) aggregatorForTenant(tenantID string) (*tenantAggregator[T], bool) {
	window := m.limits.DistributorAggregationWindow(tenantID)
	period := m.limits.DistributorAggregationPeriod(tenantID)
	if window <= 0 || period <= 0 {
		return nil, false
	}
	k := tenantKey{
		tenantID: tenantID,
		window:   window,
		period:   period,
	}
	m.m.RLock()
	t, ok := m.tenants[k]
	m.m.RUnlock()
	if ok {
		return t, true
	}

	m.m.Lock()
	defer m.m.Unlock()
	if t, ok = m.tenants[k]; ok {
		return t, true
	}

	labels := prometheus.Labels{
		"tenant_id":          tenantID,
		"aggregation_window": window.String(),
		"aggregation_period": period.String(),
	}

	a := NewAggregator[T](time.Duration(window), time.Duration(period))
	const metricNamePrefix = "pyroscope_distributor_aggregation_"
	t = &tenantAggregator[T]{
		key:        k,
		registerer: prometheus.WrapRegistererWith(labels, m.registerer),
		aggregator: a,
		collector:  NewAggregatorCollector(a, metricNamePrefix),
	}

	t.registerer.MustRegister(t.collector)
	go t.aggregator.Start()
	m.tenants[k] = t
	return t, true
}

func (m *MultiTenantAggregator[T]) starting(_ context.Context) error { return nil }

func (m *MultiTenantAggregator[T]) stopping(_ error) error {
	m.closeOnce.Do(func() {
		close(m.stop)
		<-m.done
	})
	return nil
}

const maxTenantAge = time.Second * 10

func (m *MultiTenantAggregator[T]) running(ctx context.Context) error {
	t := time.NewTicker(maxTenantAge)
	defer func() {
		t.Stop()
		close(m.done)
	}()
	for {
		select {
		case <-t.C:
			m.removeStaleTenants()
		case <-m.stop:
			return nil
		case <-ctx.Done():
			return nil
		}
	}
}

func (m *MultiTenantAggregator[T]) removeStaleTenants() {
	stale := make([]*tenantAggregator[T], len(m.tenants)/8)
	m.m.Lock()
	for _, v := range m.tenants {
		if time.Since(v.lastSeen.Load()) > maxTenantAge {
			stale = append(stale, v)
		}
	}
	m.m.Unlock()
	for _, v := range stale {
		v.aggregator.Stop()
		v.registerer.Unregister(v.collector)
		delete(m.tenants, v.key)
	}
}
