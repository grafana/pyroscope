package exporter

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

// MetricsExporter exports profiling metrics via Prometheus.
type MetricsExporter struct {
	rules []*rule
}

type rule struct {
	name string
	qry  *flameql.Query
	reg  prometheus.Registerer
	// N.B: CounterVec/MetricVec is not used due to the fact
	// that label names are not determined.
	sync.RWMutex
	counters map[uint64]prometheus.Counter
}

func NewExporter(rules []config.MetricExportRule, reg prometheus.Registerer) (*MetricsExporter, error) {
	var e MetricsExporter
	for _, c := range rules {
		qry, err := flameql.ParseQuery(c.Expr)
		if err != nil {
			return nil, err
		}
		e.rules = append(e.rules, newRule(c.MetricName, qry, reg))
	}
	return &e, nil
}

func newRule(name string, qry *flameql.Query, reg prometheus.Registerer) *rule {
	return &rule{
		name:     name,
		qry:      qry,
		reg:      reg,
		counters: make(map[uint64]prometheus.Counter),
	}
}

func (e MetricsExporter) Export(k *segment.Key, v *tree.Tree) {
	var x float64
	// TODO: find x from v lazily
	for _, r := range e.rules {
		if c, ok := r.counter(k); ok {
			c.Add(x)
		}
	}
}

// counter returns existing counter for the key or creates a new one,
// if the key satisfies the rule expression.
func (r *rule) counter(k *segment.Key) (prometheus.Counter, bool) {
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
		m = m[1:] // Remove app name label.
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

// matchedLabels returns map of KV pairs from the given key that match
// tag matchers keys of the rule regardless of their values, e.g.:
//   key:     app{foo=bar,baz=qux}
//   query:   app{foo="xxx"}
//   matched: {__name__: app, foo: bar}
//
// N.B: application name label is always first.
func (r *rule) matchedLabels(key *segment.Key) matchedLabels {
	z := matchedLabels{{flameql.ReservedTagKeyName, key.AppName()}}
	l := key.Labels()
	// Matchers may refer the same labels,
	// the set is used to filter duplicates.
	set := map[string]struct{}{}
	for _, m := range r.qry.Matchers {
		v, ok := l[m.Key]
		if !ok {
			continue
		}
		if _, ok = set[m.Key]; !ok {
			// Note that Matchers are sorted.
			z = append(z, label{m.Key, v})
			set[m.Key] = struct{}{}
		}
	}
	return z
}

// matchedLabels contain KV pairs from a dimension key that match
// tag matchers keys of a rule regardless of their values.
type matchedLabels []label

type label struct {
	key   string
	value string
}

func (m matchedLabels) hash() uint64 {
	h := fnv.New64a()
	for k, v := range m {
		_, _ = fmt.Fprint(h, k, ":", v, ";")
	}
	return binary.BigEndian.Uint64(h.Sum(nil))
}

func (m matchedLabels) labels() prometheus.Labels {
	p := make(prometheus.Labels, len(m))
	for _, l := range m {
		p[l.key] = l.value
	}
	return nil
}
