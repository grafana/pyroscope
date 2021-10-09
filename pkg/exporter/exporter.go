package exporter

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
)

// MetricsExporter exports profiling metrics via Prometheus.
// It is safe for concurrent use.
type MetricsExporter struct{ rules []*rule }

type rule struct {
	reg prometheus.Registerer

	name   string
	qry    *flameql.Query
	node   *regexp.Regexp
	labels []string

	sync.RWMutex
	counters map[uint64]prometheus.Counter
}

type observer struct {
	// Regular expressions of matched rules.
	expr []*regexp.Regexp
	// Counters corresponding matching rules.
	ctr []prometheus.Counter
	// Sample value multiplier.
	m float64
}

// NewExporter validates configuration and creates a new prometheus MetricsExporter.
// It is safe to initialize MetricsExporter without rules, then registry can be nil,
// Evaluate call will be a noop.
func NewExporter(rules config.MetricsExportRules, reg prometheus.Registerer) (*MetricsExporter, error) {
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
		var node *regexp.Regexp
		if !(r.Node == "total" || r.Node == "") {
			node, err = regexp.Compile(r.Node)
			if err != nil {
				return nil, fmt.Errorf("node must be either 'total' or a valid regexp: %w", err)
			}
		}
		if err = validateTagKeys(r.Labels); err != nil {
			return nil, fmt.Errorf("rule %q: invalid label: %w", name, err)
		}
		e.rules = append(e.rules, &rule{
			name:     name,
			qry:      qry,
			reg:      reg,
			node:     node,
			labels:   r.Labels,
			counters: make(map[uint64]prometheus.Counter),
		})
	}
	return &e, nil
}

func (e MetricsExporter) Evaluate(input *storage.PutInput) (storage.SampleObserver, bool) {
	if len(e.rules) == 0 {
		return nil, false
	}
	o := observer{m: 1}
	for _, r := range e.rules {
		c, ok := r.eval(input.Key)
		if !ok {
			continue
		}
		o.expr = append(o.expr, r.node)
		o.ctr = append(o.ctr, c)
	}
	if len(o.expr) == 0 {
		// No rules matched.
		return nil, false
	}
	if input.Units == "" {
		// Sample duration in seconds.
		o.m = 1 / float64(input.SampleRate)
	}
	return o, true
}

func (o observer) Observe(k []byte, v int) {
	if k == nil || v == 0 {
		return
	}
	for i, e := range o.expr {
		if e != nil && !e.Match(k) {
			continue
		}
		o.ctr[i].Add(float64(v) * o.m)
	}
}

// eval returns existing counter for the key or creates a new one,
// if the key satisfies the rule expression.
func (r *rule) eval(k *segment.Key) (prometheus.Counter, bool) {
	m, ok := r.matchLabelNames(k)
	if !ok {
		return nil, false
	}
	r.RLock()
	defer r.RUnlock()
	if !match(r.qry, m) {
		return nil, false
	}
	var h uint64
	if len(r.labels) == 0 {
		h = hash(k.AppName())
	} else {
		h = m.hash()
	}
	c, ok := r.counters[h]
	if ok {
		return c, true
	}
	c = prometheus.NewCounter(prometheus.CounterOpts{
		Name:        r.name,
		ConstLabels: promLabels(k, r.labels...),
	})
	r.reg.MustRegister(c)
	r.counters[h] = c
	return c, true
}

func validateTagKeys(tagKeys []string) error {
	for _, l := range tagKeys {
		if err := flameql.ValidateTagKey(l); err != nil {
			return err
		}
	}
	return nil
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

// matchLabelNames returns map of KV pairs from the given key that match
// tag matchers keys of the rule regardless of their values, e.g.:
//   key:     app{foo=bar,baz=qux}
//   query:   app{foo="xxx"}
//   matched: {__name__: app, foo: bar}
//
// The key must include labels required by the rule expression, otherwise
// the function returns empty labels and false.
func (r *rule) matchLabelNames(key *segment.Key) (labels, bool) {
	appName := key.AppName()
	if appName != r.qry.AppName {
		return nil, false
	}
	// This is required for a case when there are no tag matchers.
	z := labels{{flameql.ReservedTagKeyName, appName}}
	l := key.Labels()
	// If no matchers specified (only application name),
	// all the allowed labels are considered matched.
	if len(r.qry.Matchers) == 0 {
		for _, k := range r.labels {
			if v, ok := l[k]; ok {
				z = append(z, label{k, v})
			}
		}
		return z, true
	}
	// Matchers may refer the same labels, duplicates should be removed.
	set := map[string]struct{}{}
	for _, m := range r.qry.Matchers {
		v, ok := l[m.Key]
		if !ok {
			// If the matcher label is required (e.g. the matcher operator
			// is OpEqual or OpEqualRegex) but not present, return.
			if m.IsNegation() {
				continue
			}
			return nil, false
		}
		if _, ok = set[m.Key]; !ok {
			z = append(z, label{m.Key, v})
			set[m.Key] = struct{}{}
		}
	}
	return z, true
}

// labels contain KV label pairs from a segment key.
type labels []label

type label struct{ key, value string }

func hash(v string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(v))
	return h.Sum64()
}

// hash returns FNV-1a hash of labels key value pairs.
func (l labels) hash() uint64 {
	h := fnv.New64a()
	for _, x := range l {
		_, _ = h.Write([]byte(x.key + ":" + x.value))
	}
	return h.Sum64()
}
