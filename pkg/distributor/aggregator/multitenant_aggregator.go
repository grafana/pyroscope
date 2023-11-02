package aggregator

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"
)

type MultiTenantAggregator[T any] struct {
	limits     Limits
	registerer prometheus.Registerer

	m       sync.RWMutex
	tenants map[tenantKey]*tenantAggregator[T]
}

type Limits interface {
	DistributorAggregationWindow(tenantID string) time.Duration
	DistributorAggregationPeriod(tenantID string) time.Duration
}

func NewMultiTenantAggregator[T any](limits Limits, registerer prometheus.Registerer) *MultiTenantAggregator[T] {
	return &MultiTenantAggregator[T]{
		limits:     limits,
		registerer: registerer,
		tenants:    make(map[tenantKey]*tenantAggregator[T]),
	}
}

type tenantAggregator[T any] struct {
	lastSeen   atomic.Time
	key        tenantKey
	registerer prometheus.Registerer
	aggregator *Aggregator[T]
}

type tenantKey struct {
	tenantID string
	window   time.Duration
	period   time.Duration
}

// TODO(kolesnikovae): Shutdown inactive aggregators.

func (m *MultiTenantAggregator[T]) AggregatorForTenant(tenantID string) (*Aggregator[T], bool) {
	window := m.limits.DistributorAggregationWindow(tenantID)
	period := m.limits.DistributorAggregationPeriod(tenantID)
	if window == 0 || period == 0 {
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
	defer t.lastSeen.Store(time.Now())
	if ok {
		return t.aggregator, true
	}

	m.m.Lock()
	defer m.m.Unlock()
	if t, ok = m.tenants[k]; ok {
		return t.aggregator, true
	}

	labels := prometheus.Labels{
		"tenant_id":          tenantID,
		"aggregation_window": window.String(),
		"aggregation_period": period.String(),
	}

	t = &tenantAggregator[T]{
		key:        k,
		registerer: prometheus.WrapRegistererWith(labels, m.registerer),
		aggregator: NewAggregator[T](window, period),
	}

	RegisterAggregatorCollector(t.aggregator, t.registerer)
	t.aggregator.Start()
	m.tenants[k] = t

	return t.aggregator, true
}
