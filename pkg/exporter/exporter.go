package exporter

import (
	"fmt"
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

// MetricsExporter exports profiling metrics via Prometheus.
// Safe for concurrent use.
type MetricsExporter struct{ rules []*rule }

type rule struct {
	reg prometheus.Registerer

	name string
	qry  *flameql.Query
	node

	// N.B: CounterVec/MetricVec is not used due to the fact
	// that label names are not determined.
	sync.RWMutex
	counters map[uint64]prometheus.Counter
}

// NewExporter validates configuration and creates a new prometheus MetricsExporter.
func NewExporter(rules []config.MetricExportRule, reg prometheus.Registerer) (*MetricsExporter, error) {
	var e MetricsExporter
	for _, c := range rules {
		// TODO(kolesnikovae): validate metric name.
		qry, err := flameql.ParseQuery(c.Expr)
		if err != nil {
			return nil, fmt.Errorf("rule %q: invalid expression %q: %w", c.Name, c.Expr, err)
		}
		n, err := newNode(c.Node)
		if err != nil {
			return nil, fmt.Errorf("rule %q: invalid node %q: %w", c.Name, c.Node, err)
		}
		e.rules = append(e.rules, &rule{
			name:     c.Name,
			qry:      qry,
			reg:      reg,
			node:     n,
			counters: make(map[uint64]prometheus.Counter),
		})
	}
	return &e, nil
}

// Observe ingested key and value.
//
// The call evaluates export rules against the key k and creates prometheus
// counters for new time series, if required. Every export rule has an
// expression to evaluate a dimension key, and a filter, which allow to
// retrieve metric value for particular nodes.
//
// When a new counter is created, labels matching the rule expression are
// preserved. Therefore it is crucial to keep query cardinality low.
func (e MetricsExporter) Observe(k *segment.Key, tree *tree.Tree) {
	for _, r := range e.rules {
		c, ok := r.eval(k)
		if !ok {
			continue
		}
		if val, ok := r.value(tree); ok {
			c.Add(val)
		}
	}
}

// eval returns existing counter for the key or creates a new one,
// if the key satisfies the rule expression.
func (r *rule) eval(k *segment.Key) (prometheus.Counter, bool) {
	if k.AppName() != r.qry.AppName {
		return nil, false
	}
	m := r.matchedLabels(k)
	h := m.hash()
	r.RLock()
	c, ok := r.counters[h]
	if ok {
		r.RUnlock()
		return c, true
	}
	r.RUnlock()
	if match(r.qry, m) {
		// Remove app name label to avoid
		// collision with prometheus labels.
		m = m[1:]
		c = prometheus.NewCounter(prometheus.CounterOpts{
			Name:        r.name,
			ConstLabels: m.labels(),
		})
		r.reg.MustRegister(c)
		r.Lock()
		r.counters[h] = c
		r.Unlock()
		return c, true
	}
	return nil, false
}

// match reports whether the key matches the query.
func match(qry *flameql.Query, labels matchedLabels) bool {
	for _, m := range qry.Matchers {
		var ok bool
		for _, l := range labels {
			if m.Key != l.key {
				continue
			}
			if m.Match(l.value) {
				if !m.IsNegation() {
					ok = true
					break
				}
			} else if m.IsNegation() {
				return false
			}
		}
		if !ok && !m.IsNegation() {
			return false
		}
	}
	return true
}
