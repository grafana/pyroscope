package exporter

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

func TestObserve(t *testing.T) {
	rules := config.MetricsExportRules{
		"app_name_cpu_total": {
			Expr: `app.name.cpu`,
		},
		"app_name_cpu_total_foo": {
			Expr:    `app.name.cpu{foo="bar"}`,
			Node:    "total",
			GroupBy: []string{"foo"},
		},
		"app_name_cpu_abc": {
			Expr:    `app.name.cpu{foo=~"b.*"}`,
			Node:    "^a;b;c$",
			GroupBy: []string{"foo"},
		},
		"app_name_cpu_ab": {
			Expr:    `app.name.cpu{foo=~"b.*"}`,
			Node:    "a;b",
			GroupBy: []string{"foo"},
		},
		"another_app": {
			Expr:    `another.app.cpu`,
			GroupBy: []string{"foo"},
		},
	}

	exporter, _ := NewExporter(rules, prometheus.NewRegistry())
	k := observe(exporter, "app.name.cpu{foo=bar,bar=baz}")

	requireRuleCounterValue(t, exporter, "app_name_cpu_total", k, 5)
	requireRuleCounterValue(t, exporter, "app_name_cpu_total_foo", k, 5)
	requireRuleCounterValue(t, exporter, "app_name_cpu_abc", k, 2)
	requireRuleCounterValue(t, exporter, "app_name_cpu_ab", k, 3)
	requireNoCounter(t, exporter, "another_app", k)
}

const testRuleName = "app_name_cpu_total"

func TestObserveNoMatch(t *testing.T) {
	rules := config.MetricsExportRules{
		testRuleName: {
			Expr: `app.name.cpu`,
		},
	}

	exporter, _ := NewExporter(rules, prometheus.NewRegistry())
	k := observe(exporter, "another.app.name.cpu{foo=bar,bar=baz}")

	requireNoCounter(t, exporter, testRuleName, k)
}

func TestGroupBy(t *testing.T) {
	rules := config.MetricsExportRules{
		testRuleName: {
			Expr:    `app.name.cpu`,
			GroupBy: []string{"foo"},
		},
	}

	exporter, _ := NewExporter(rules, prometheus.NewRegistry())
	k1 := observe(exporter, `app.name.cpu{foo=bar_a,bar=a}`)
	k2 := observe(exporter, `app.name.cpu{foo=bar_a,bar=b}`)
	k3 := observe(exporter, `app.name.cpu{foo=bar_b,bar=c}`)
	k4 := observe(exporter, `app.name.cpu{}`)

	requireRuleCounterValue(t, exporter, testRuleName, k1, 10)
	requireRuleCounterValue(t, exporter, testRuleName, k2, 10)
	requireRuleCounterValue(t, exporter, testRuleName, k3, 5)
	requireRuleCounterValue(t, exporter, testRuleName, k4, 5)
}

func TestNoGroupBy(t *testing.T) {
	rules := config.MetricsExportRules{
		testRuleName: {
			Expr: `app.name.cpu`,
		},
	}

	exporter, _ := NewExporter(rules, prometheus.NewRegistry())
	k1 := observe(exporter, "app.name.cpu{foo=bar}")
	k2 := observe(exporter, "app.name.cpu{foo=baz}")

	requireRuleCounterValue(t, exporter, testRuleName, k1, 10)
	requireRuleCounterValue(t, exporter, testRuleName, k2, 10)
}

func TestSampleUnits(t *testing.T) {
	rules := config.MetricsExportRules{
		testRuleName: {Expr: "app.name.cpu"},
	}

	exporter, _ := NewExporter(rules, prometheus.NewRegistry())
	k, _ := segment.ParseKey("app.name.cpu")
	o, _ := exporter.Evaluate(&storage.PutInput{Key: k, SampleRate: 100})
	createTree().Iterate(observeCallback(o))

	requireRuleCounterValue(t, exporter, testRuleName, k, 0.05)
}

func getCounter(e *MetricsExporter, name string, k *segment.Key) prometheus.Counter {
	r, ok := e.rules[name]
	if !ok {
		return nil
	}
	return r.counterForKey(k)
}

func requireNoCounter(t *testing.T, e *MetricsExporter, name string, k *segment.Key) {
	r, ok := e.rules[name]
	if !ok || r.ctr.Delete(r.promLabels(k)) {
		t.Fatalf("Unexpected counter %s (%v)", name, k)
	}
}

func requireRuleCounterValue(t *testing.T, e *MetricsExporter, name string, k *segment.Key, v float64) {
	if actual := testutil.ToFloat64(getCounter(e, name, k)); v != actual {
		t.Fatalf("Expected value %v got %v; counter %s (%v)", v, actual, name, k)
	}
}

func observe(e *MetricsExporter, key string) *segment.Key {
	k, _ := segment.ParseKey(key)
	if o, ok := e.Evaluate(&storage.PutInput{Key: k, Units: "samples"}); ok {
		createTree().Iterate(observeCallback(o))
	}
	return k
}

func createTree() *tree.Tree {
	t := tree.New()
	t.Insert([]byte("a;b"), uint64(1))
	t.Insert([]byte("a;c"), uint64(2))
	t.Insert([]byte("a;b;c"), uint64(2))
	return t
}

func observeCallback(o storage.SampleObserver) func([]byte, uint64) {
	return func(key []byte, val uint64) {
		// Key has ;; prefix.
		if len(key) > 2 && val != 0 {
			o.Observe(key[2:], int(val))
		}
	}
}
