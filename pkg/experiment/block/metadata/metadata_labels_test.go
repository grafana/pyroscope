package metadata

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/model"
)

func TestLabelBuilder_CreateLabels(t *testing.T) {
	strings := NewStringTable()
	b := NewLabelBuilder(strings).
		WithConstantPairs("foo", "0").
		WithLabelNames("bar", "baz")

	b.CreateLabels("1", "2")
	b.CreateLabels("1", "2")
	b.CreateLabels("3", "4")

	assert.Equal(t, []int32{
		3, 1, 2, 3, 5, 4, 6, // foo=0, bar=1, baz=2
		3, 1, 2, 3, 5, 4, 6, // foo=0, bar=1, baz=2
		3, 1, 2, 3, 7, 4, 8, // foo=0, bar=3, baz=4
	}, b.Build())

	assert.EqualValues(t, 5, strings.LookupString("1"))
	assert.EqualValues(t, 6, strings.LookupString("2"))
	assert.EqualValues(t, 7, strings.LookupString("3"))
	assert.EqualValues(t, 8, strings.LookupString("4"))
}

func TestLabelBuilder_Reuse(t *testing.T) {
	strings := NewStringTable()
	b := NewLabelBuilder(strings).
		WithConstantPairs("service_name", "service_a").
		WithLabelNames("__profile_type__")

	b.CreateLabels("cpu:a")
	b.CreateLabels("cpu:b")
	b.CreateLabels("memory")
	assert.Equal(t, []string{
		"service_name=service_a;__profile_type__=cpu:a;",
		"service_name=service_a;__profile_type__=cpu:b;",
		"service_name=service_a;__profile_type__=memory;",
	}, labelStrings(b.Build(), strings))

	b.WithConstantPairs("service_name", "service_b")
	assert.True(t, b.CreateLabels("cpu:a"))
	assert.Equal(t, []string{
		"service_name=service_b;__profile_type__=cpu:a;",
	}, labelStrings(b.Build(), strings))

	b = b.WithLabelNames("another_label")
	b.CreateLabels("another_value")
	assert.Equal(t, []string{
		"service_name=service_b;another_label=another_value;",
	}, labelStrings(b.Build(), strings))
}

func TestLabelBuilder_Put(t *testing.T) {
	strings := NewStringTable()
	b := NewLabelBuilder(strings)

	// a=b, a=b; a=b, a=b;
	b.Put([]int32{2, 1, 2, 1, 2, 2, 1, 2, 1, 2}, []string{"", "a", "b"})
	b.Put([]int32{2, 1, 2, 1, 2, 2, 1, 2, 1, 2}, []string{"", "a", "b"})

	// c=d, c=d; c=d, c=d;
	b.Put([]int32{2, 1, 2, 1, 2, 2, 1, 2, 1, 2}, []string{"", "c", "d"})
	b.Put([]int32{2, 1, 2, 1, 2}, []string{"", "c", "d"})

	assert.Equal(t, []int32{
		2, 1, 2, 1, 2,
		2, 3, 4, 3, 4,
	}, b.Build())
}

func labelStrings(v []int32, s *StringTable) []string {
	var ls []string
	pairs := LabelPairs(v)
	for pairs.Next() {
		p := pairs.At()
		var l string
		for len(p) > 0 {
			l += s.Lookup(p[0]) + "=" + s.Lookup(p[1]) + ";"
			p = p[2:]
		}
		ls = append(ls, l)
	}
	return ls
}

func TestLabelMatcher_Matches(t *testing.T) {
	strings := NewStringTable()
	b := NewLabelBuilder(strings)

	b.WithConstantPairs("service_name", "service_a")
	b.WithLabelNames("__profile_type__")
	b.CreateLabels("cpu:a")
	b.CreateLabels("cpu:b")
	b.CreateLabels("memory")
	setA := b.Build()
	assert.Equal(t, []string{
		"service_name=service_a;__profile_type__=cpu:a;",
		"service_name=service_a;__profile_type__=cpu:b;",
		"service_name=service_a;__profile_type__=memory;",
	}, labelStrings(setA, strings))

	b.WithConstantPairs("service_name", "service_b")
	b.CreateLabels("cpu:a")
	b.CreateLabels("cpu:b")
	setB := b.Build()
	assert.Equal(t, []string{
		"service_name=service_b;__profile_type__=cpu:a;",
		"service_name=service_b;__profile_type__=cpu:b;",
	}, labelStrings(setB, strings))

	m := NewLabelMatcher(strings, []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, "service_name", "service_a"),
		labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "cpu:a")},
		"service_name",
		"__profile_type__",
		"none")
	assert.True(t, m.IsValid())

	expected := []bool{true, false, false, false, false}
	matches := make([]bool, 0, len(expected))

	pairs := LabelPairs(setA)
	for pairs.Next() {
		matches = append(matches, m.MatchesPairs(pairs.At()))
	}

	pairs = LabelPairs(setB)
	for pairs.Next() {
		matches = append(matches, m.MatchesPairs(pairs.At()))
	}

	assert.Equal(t, expected, matches)
	assert.Equal(t, []model.Labels{{
		&typesv1.LabelPair{Name: "service_name", Value: "service_a"},
		&typesv1.LabelPair{Name: "__profile_type__", Value: "cpu:a"},
		&typesv1.LabelPair{Name: "none", Value: ""},
	}}, m.AllMatches())
}

func Test_LabelMatcher_All(t *testing.T) {
	strings := NewStringTable()
	x := NewLabelBuilder(strings).BuildPairs(
		LabelNameTenantDataset,
		LabelValueDatasetTSDBIndex,
	)

	m := NewLabelMatcher(strings,
		[]*labels.Matcher{},
		"service_name",
		"__profile_type__",
	)

	assert.True(t, m.IsValid())
	assert.True(t, m.Matches(x))
	assert.Equal(t, []model.Labels{{
		&typesv1.LabelPair{Name: "service_name", Value: ""},
		&typesv1.LabelPair{Name: "__profile_type__", Value: ""},
	}}, m.AllMatches())
}

func TestLabelMatcher_Collect(t *testing.T) {
	strings := NewStringTable()
	b := NewLabelBuilder(strings)

	b.WithConstantPairs("service_name", "service_a")
	b.WithLabelNames("__profile_type__")
	b.CreateLabels("cpu:a")
	b.CreateLabels("cpu:b")
	b.CreateLabels("memory")
	setA := b.Build()
	assert.Equal(t, []string{
		"service_name=service_a;__profile_type__=cpu:a;",
		"service_name=service_a;__profile_type__=cpu:b;",
		"service_name=service_a;__profile_type__=memory;",
	}, labelStrings(setA, strings))

	b.WithConstantPairs("service_name", "service_b")
	b.CreateLabels("cpu:a")
	b.CreateLabels("cpu:b")
	setB := b.Build()
	assert.Equal(t, []string{
		"service_name=service_b;__profile_type__=cpu:a;",
		"service_name=service_b;__profile_type__=cpu:b;",
	}, labelStrings(setB, strings))

	m := NewLabelMatcher(strings, []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, "service_name", "service_a"),
		labels.MustNewMatcher(labels.MatchRegexp, "__profile_type__", "cpu.*")},
		"service_name",
		"none")
	assert.True(t, m.IsValid())

	matches, ok := m.CollectMatches(nil, setA)
	assert.True(t, ok)
	assert.Equal(t, []string{
		"service_name=service_a;",
		"service_name=service_a;",
	}, labelStrings(matches, strings))

	matches = matches[:0]
	matches, ok = m.CollectMatches(matches, setB)
	assert.False(t, ok)
	assert.Len(t, matches, 0)
}

func Benchmark_LabelMatcher_Matches(b *testing.B) {
	strings := NewStringTable()

	lb := NewLabelBuilder(strings).
		WithConstantPairs("service_name", "service_a").
		WithLabelNames("__profile_type__")
	lb.CreateLabels("cpu")
	ls := lb.Build()

	m := NewLabelMatcher(strings,
		[]*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "service_name", "service_a")},
		"service_name", "__profile_type__")

	assert.True(b, m.IsValid())
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		pairs := LabelPairs(ls)
		for pairs.Next() {
			m.MatchesPairs(pairs.At())
		}
	}
}
