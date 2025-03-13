package metadata

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/model"
)

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
	setA := NewLabelBuilder(strings).
		WithLabelSet("service_name", "service_a", "__profile_type__", "cpu:a").
		WithLabelSet("service_name", "service_a", "__profile_type__", "cpu:b").
		WithLabelSet("service_name", "service_a", "__profile_type__", "memory").
		Build()
	assert.Equal(t, []string{
		"service_name=service_a;__profile_type__=cpu:a;",
		"service_name=service_a;__profile_type__=cpu:b;",
		"service_name=service_a;__profile_type__=memory;",
	}, labelStrings(setA, strings))

	setB := NewLabelBuilder(strings).
		WithLabelSet("service_name", "service_b", "__profile_type__", "cpu:a").
		WithLabelSet("service_name", "service_b", "__profile_type__", "cpu:b").
		Build()
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
	x := NewLabelBuilder(strings).
		WithLabelSet(LabelNameTenantDataset, LabelValueDatasetTSDBIndex).
		Build()

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
	setA := NewLabelBuilder(strings).
		WithLabelSet("service_name", "service_a", "__profile_type__", "cpu:a").
		WithLabelSet("service_name", "service_a", "__profile_type__", "cpu:b").
		WithLabelSet("service_name", "service_a", "__profile_type__", "memory").
		Build()

	setB := NewLabelBuilder(strings).
		WithLabelSet("service_name", "service_b", "__profile_type__", "cpu:a").
		WithLabelSet("service_name", "service_b", "__profile_type__", "cpu:b").
		Build()

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

	ls := NewLabelBuilder(strings).
		WithLabelSet("service_name", "service_a", "__profile_type__", "cpu").
		Build()

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

func TestFindDatasets(t *testing.T) {
	strings := NewStringTable()
	setA := NewLabelBuilder(strings).
		WithLabelSet("service_name", "service_a", "__profile_type__", "cpu:a").
		WithLabelSet("service_name", "service_a", "__profile_type__", "cpu:b").
		WithLabelSet("service_name", "service_a", "__profile_type__", "memory").
		Build()

	setB := NewLabelBuilder(strings).
		WithLabelSet("service_name", "service_b", "__profile_type__", "cpu:a").
		WithLabelSet("service_name", "service_b", "__profile_type__", "cpu:b").
		Build()

	md := &metastorev1.BlockMeta{
		Datasets: []*metastorev1.Dataset{
			{Name: 3, Labels: setA},
			{Name: 4, Labels: setB},
		},
		StringTable: strings.Strings,
	}

	for _, test := range []struct {
		matchers []*labels.Matcher
		expected []int32
	}{
		{
			expected: []int32{3, 4},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "foo", "bar")},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "service_name", "service_b")},
			expected: []int32{4},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotEqual, "service_name", "service_b")},
			expected: []int32{3},
		},
		{
			matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "service_name", ".*")},
			expected: []int32{3, 4},
		},
		{
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "memory"),
			},
			expected: []int32{3},
		},
		{
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "memory"),
				labels.MustNewMatcher(labels.MatchEqual, "service_name", "service_a"),
			},
			expected: []int32{3},
		},
		{
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "memory"),
				labels.MustNewMatcher(labels.MatchEqual, "service_name", "service_b"),
			},
		},
	} {
		var actual []int32
		FindDatasets(md, test.matchers...)(func(v *metastorev1.Dataset) bool {
			actual = append(actual, v.Name)
			return true
		})
		assert.Equal(t, test.expected, actual)
	}
}
