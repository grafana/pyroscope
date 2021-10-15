package exporter

import (
	"fmt"
	"regexp"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
)

// MetricsExporter exports profiling metrics via Prometheus.
// It is safe for concurrent use.
type MetricsExporter struct{ rules map[string]*rule }

type rule struct {
	name   string
	qry    *flameql.Query
	node   *regexp.Regexp
	labels []string
	ctr    *prometheus.CounterVec
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
	e := MetricsExporter{
		rules: make(map[string]*rule),
	}
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
		if err = validateTagKeys(r.GroupBy); err != nil {
			return nil, fmt.Errorf("rule %q: invalid label: %w", name, err)
		}
		c := prometheus.NewCounterVec(prometheus.CounterOpts{Name: name}, r.GroupBy)
		if err = reg.Register(c); err != nil {
			return nil, err
		}
		e.rules[name] = &rule{
			qry:    qry,
			node:   node,
			labels: r.GroupBy,
			ctr:    c,
		}
	}
	return &e, nil
}

func (e MetricsExporter) Evaluate(input *storage.PutInput) (storage.SampleObserver, bool) {
	if len(e.rules) == 0 {
		return nil, false
	}
	o := observer{m: 1}
	for _, r := range e.rules {
		if !input.Key.Match(r.qry) {
			continue
		}
		o.expr = append(o.expr, r.node)
		o.ctr = append(o.ctr, r.ctr.With(r.promLabels(input.Key)))
	}
	if len(o.expr) == 0 {
		// No rules matched.
		return nil, false
	}
	if input.Units == spy.ProfileCPU.Units() {
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
func (r rule) promLabels(key *segment.Key) prometheus.Labels {
	if len(r.labels) == 0 {
		return nil
	}
	// labels are guarantied to be valid.
	l := key.Labels()
	p := make(prometheus.Labels, len(r.labels))
	for _, k := range r.labels {
		p[k] = l[k]
	}
	return p
}
