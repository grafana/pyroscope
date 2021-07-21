package exporter

import (
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

func TestExport(t *testing.T) {
	rules := []config.MetricExportRule{
		{
			Name: "app_name_cpu_total",
			Expr: `app.name.cpu{foo="bar"}`,
			Node: "total",
		},
		{
			Name: "app_name_cpu_abc",
			Expr: `app.name.cpu{foo=~"b.*"}`,
			Node: "^a;b;c$",
		},
	}

	exporter, _ := NewExporter(rules, prometheus.NewRegistry())
	k, _ := segment.ParseKey(`app.name.cpu{foo=bar,bar=baz}`)
	v := createTree()
	exporter.Observe(k, v)

	// Hashes are predetermined.
	total := testutil.ToFloat64(exporter.rules[0].counters[16252301464360304376])
	if total != 5 {
		t.Fatalf("total counter must be 5")
	}

	abc := testutil.ToFloat64(exporter.rules[1].counters[16252301464360304376])
	if abc != 2 {
		t.Fatalf("a;b;c counter must be 2")
	}
}

func createTree() *tree.Tree {
	t := tree.New()
	t.Insert([]byte("a;b"), uint64(1))
	t.Insert([]byte("a;c"), uint64(2))
	t.Insert([]byte("a;b;c"), uint64(2))
	return t
}
