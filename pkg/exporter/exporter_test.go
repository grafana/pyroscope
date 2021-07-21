package exporter

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
)

func TestMatch(t *testing.T) {
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
		k, _ := segment.ParseKey(tc.key)
		qry, _ := flameql.ParseQuery(tc.query)
		r := newRule("test", qry, prometheus.NewRegistry())
		_, matched := r.counter(k)
		if matched != tc.match {
			t.Fatalf("Expect matches: %v, actual: %v\n\tQuery: %s\n\tKey: %v", tc.match, matched, tc.query, tc.key)
		}
	}
}

// MustRegister causes panic if the same counter (name+labels) is registered
// twice. The test is aimed to ensure that a counter is created once per unique
// labels set (matching query).
func TestRegister(t *testing.T) {
	qry, _ := flameql.ParseQuery(`app.name{foo=~"bar"}`)
	reg := prometheus.NewRegistry()
	r := newRule("test", qry, reg)
	k, _ := segment.ParseKey(`app.name{foo=barbar,bar=bar}`)
	r.counter(k)
	r.counter(k)
	if len(r.counters) != 1 {
		t.Fatalf("Expected exactly one counter, got %d", len(r.counters))
	}
}
