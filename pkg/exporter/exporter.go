package exporter

import (
	"fmt"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

// MetricsExporter exports profiling metrics via Prometheus.
// It is safe for concurrent use.
type MetricsExporter struct{ rules []*rule }

type rule struct {
	reg prometheus.Registerer

	name   string
	qry    *flameql.Query
	labels []string
	node

	sync.RWMutex
	counters map[uint64]prometheus.Counter
}

// NewExporter validates configuration and creates a new prometheus MetricsExporter.
func NewExporter(rules config.MetricExportRules, reg prometheus.Registerer) (*MetricsExporter, error) {
	var e MetricsExporter
	if rules == nil {
		return &e, nil
	}
	for name, r := range rules {
		if !model.IsValidMetricName(model.LabelValue(name)) {
			return nil, fmt.Errorf("%q is not a valid metric name", name)
		}
		qry, err := flameql.ParseQuery(r.Expr)
		if err != nil {
			return nil, fmt.Errorf("rule %q: invalid expression %q: %w", name, r.Expr, err)
		}
		n, err := newNode(r.Node)
		if err != nil {
			return nil, fmt.Errorf("rule %q: invalid node %q: %w", name, r.Node, err)
		}
		g, err := validateTagKeys(r.Labels)
		if err != nil {
			return nil, fmt.Errorf("rule %q: invalid label: %w", name, err)
		}
		e.rules = append(e.rules, &rule{
			name:     name,
			qry:      qry,
			reg:      reg,
			node:     n,
			labels:   g,
			counters: make(map[uint64]prometheus.Counter),
		})
	}
	return &e, nil
}

// Observe ingested segment key and tree.
//
// The call evaluates export rules against the key k and creates prometheus
// counters for new time series, if required. Every export rule has an
// expression to evaluate a dimension key, and a filter, which allow to
// retrieve metric value for particular nodes.
func (e MetricsExporter) Observe(k *segment.Key, tree *tree.Tree, multiplier float64) {
	for _, r := range e.rules {
		c, ok := r.eval(k)
		if !ok {
			continue
		}
		if val, ok := r.value(tree); ok {
			c.Add(val * multiplier)
		}
	}
}

// eval returns existing counter for the key or creates a new one,
// if the key satisfies the rule expression.
func (r *rule) eval(k *segment.Key) (prometheus.Counter, bool) {
	m, ok := r.matchLabelNames(k)
	if !ok {
		return nil, false
	}
	h := m.hash()
	r.RLock()
	c, ok := r.counters[h]
	if ok {
		r.RUnlock()
		return c, true
	}
	r.RUnlock()
	if match(r.qry, m) {
		c = prometheus.NewCounter(prometheus.CounterOpts{
			Name:        r.name,
			ConstLabels: promLabels(k, r.labels...),
		})
		r.reg.MustRegister(c)
		r.Lock()
		r.counters[h] = c
		r.Unlock()
		return c, true
	}
	return nil, false
}

func validateTagKeys(tagKeys []string) ([]string, error) {
	for _, l := range tagKeys {
		if err := flameql.ValidateTagKey(l); err != nil {
			return nil, err
		}
	}
	return tagKeys, nil
}

// promLabels converts key to prometheus.Labels ignoring reserved tag keys.
// Only explicitly listed labels are converted.
func promLabels(key *segment.Key, labels ...string) prometheus.Labels {
	if len(labels) == 0 {
		return nil
	}
	l := key.Labels()
	p := make(prometheus.Labels, len(labels))
	// labels are guarantied to be valid.
	for _, k := range labels {
		if v, ok := l[k]; ok {
			p[k] = v
		}
	}
	return p
}

// match reports whether the key matches the query.
func match(qry *flameql.Query, labels labels) bool {
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
