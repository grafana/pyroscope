package exporter

import (
	"reflect"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

func TestEval(t *testing.T) {
	type testCase struct {
		query string
		match bool
		key   string
	}

	testCases := []testCase{
		// No matchers specified except app name.
		{`app.name`, true, `app.name{foo=bar}`},
		{`app.name{}`, true, `app.name{foo=bar}`},
		{`app.name`, false, `x.name{foo=bar}`},
		{`app.name{}`, false, `x.name{foo=bar}`},

		{`app.name{foo="bar"}`, true, `app.name{foo=bar}`},
		{`app.name{foo!="bar"}`, true, `app.name{foo=baz}`},
		{`app.name{foo="bar"}`, false, `app.name{foo=baz}`},
		{`app.name{foo!="bar"}`, false, `app.name{foo=bar}`},

		// Tag key not present.
		{`app.name{foo="bar"}`, false, `app.name{bar=baz}`},
		{`app.name{foo!="bar"}`, true, `app.name{bar=baz}`},

		{`app.name{foo="bar",baz="qux"}`, true, `app.name{foo=bar,baz=qux}`},
		{`app.name{foo="bar",baz!="qux"}`, true, `app.name{foo=bar,baz=fred}`},
		{`app.name{foo="bar",baz="qux"}`, false, `app.name{foo=bar}`},
		{`app.name{foo="bar",baz!="qux"}`, false, `app.name{foo=bar,baz=qux}`},
		{`app.name{foo="bar",baz!="qux"}`, false, `app.name{baz=fred,bar=baz}`},
	}

	for _, tc := range testCases {
		qry, _ := flameql.ParseQuery(tc.query)
		k, _ := segment.ParseKey(tc.key)
		r := &rule{
			qry:      qry,
			name:     "test",
			counters: make(map[uint64]prometheus.Counter),
			reg:      prometheus.NewRegistry(),
		}

		_, matched := r.eval(k)
		if matched != tc.match {
			t.Fatalf("Expect matches: %v, actual: %v\n\tQuery: %s\n\tKey: %v",
				tc.match, matched, tc.query, tc.key)
		}
	}
}

// MustRegister causes panic if the same counter (name+labels) is registered
// twice. The test is aimed to ensure that a counter is created once per unique
// labels set (matching query).
func TestRegister(t *testing.T) {
	qry, _ := flameql.ParseQuery(`app.name{foo=~"bar"}`)
	k, _ := segment.ParseKey(`app.name{foo=barbar,bar=bar}`)
	r := &rule{
		qry:      qry,
		name:     "test",
		counters: make(map[uint64]prometheus.Counter),
		reg:      prometheus.NewRegistry(),
	}

	r.eval(k)
	r.eval(k)

	if len(r.counters) != 1 {
		t.Fatalf("Expected exactly one counter, got %d", len(r.counters))
	}
}

func TestObserve(t *testing.T) {
	rules := config.MetricExportRules{
		"app_name_cpu_total": {
			Expr:   `app.name.cpu{foo="bar"}`,
			Node:   "total",
			Labels: []string{"foo"},
		},
		"app_name_cpu_abc": {
			Expr:   `app.name.cpu{foo=~"b.*"}`,
			Node:   "^a;b;c$",
			Labels: []string{"foo"},
		},
		"app_name_cpu_ab": {
			Expr:   `app.name.cpu{foo=~"b.*"}`,
			Node:   "a;b",
			Labels: []string{"foo"},
		},
	}

	exporter, _ := NewExporter(rules, prometheus.NewRegistry())
	k, _ := segment.ParseKey(`app.name.cpu{foo=bar,bar=baz}`)
	v := createTree()
	exporter.Observe(k, v, 1)

	if total := getRuleCounterValue(exporter, "app_name_cpu_total", k); total != 5 {
		t.Fatalf("Total counter must be 5, got %v", total)
	}

	if abc := getRuleCounterValue(exporter, "app_name_cpu_abc", k); abc != 2 {
		t.Fatalf("a;b;c counter must be 2, got %v", abc)
	}

	if ab := getRuleCounterValue(exporter, "app_name_cpu_ab", k); ab != 3 {
		t.Fatalf("a;b counter must be 3, got %v", ab)
	}
}

func TestGroupBy(t *testing.T) {
	const rule = "app_name_cpu_total"
	rules := config.MetricExportRules{
		rule: {
			Expr:   `app.name.cpu{foo=~"bar"}`,
			Labels: []string{"foo"},
		},
	}

	exporter, _ := NewExporter(rules, prometheus.NewRegistry())
	k1 := observe(exporter, `app.name.cpu{foo=bar_a,bar=a}`)
	k2 := observe(exporter, `app.name.cpu{foo=bar_a,bar=b}`)
	k3 := observe(exporter, `app.name.cpu{foo=bar_b,bar=c}`)

	counters := len(exporter.rules[0].counters)
	if counters != 2 {
		t.Fatalf("Expected 2 counters, got %v", counters)
	}

	c1 := getRuleCounter(exporter, rule, k1)
	c2 := getRuleCounter(exporter, rule, k2)
	c3 := getRuleCounter(exporter, rule, k3)

	if !reflect.DeepEqual(c1, c2) {
		t.Fatalf("Expected c1 and c2 is the same counter")
	}

	if t1 := testutil.ToFloat64(c1); t1 != 10 {
		t.Fatalf("Total counter for k1 must be 10, got %v", t1)
	}

	if t2 := testutil.ToFloat64(c2); t2 != 10 {
		t.Fatalf("Total counter for k2 must be 10, got %v", t2)
	}

	if t3 := testutil.ToFloat64(c3); t3 != 5 {
		t.Fatalf("Total counter for k3 must be 5, got %v", t3)
	}
}

func getRuleCounter(e *MetricsExporter, name string, k *segment.Key) prometheus.Counter {
	for _, r := range e.rules {
		if r.name != name {
			continue
		}
		m, ok := r.matchLabelNames(k)
		if !ok {
			continue
		}
		if c, ok := r.counters[m.hash()]; ok {
			return c
		}
	}
	return nil
}

func getRuleCounterValue(e *MetricsExporter, name string, k *segment.Key) float64 {
	return testutil.ToFloat64(getRuleCounter(e, name, k))
}

func observe(e *MetricsExporter, key string) *segment.Key {
	k, _ := segment.ParseKey(key)
	e.Observe(k, createTree(), 1)
	return k
}

func createTree() *tree.Tree {
	t := tree.New()
	t.Insert([]byte("a;b"), uint64(1))
	t.Insert([]byte("a;c"), uint64(2))
	t.Insert([]byte("a;b;c"), uint64(2))
	return t
}
