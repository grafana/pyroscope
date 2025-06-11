package metadata

import (
	goiter "iter"
	"slices"
	"strings"
	"unsafe"

	"github.com/prometheus/prometheus/model/labels"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/iter"
)

// TODO(kolesnikovae): LabelBuilder pool.

const (
	LabelNameTenantDataset     = "__tenant_dataset__"
	LabelValueDatasetTSDBIndex = "dataset_tsdb_index"
	LabelNameUnsymbolized      = "__unsymbolized__"
)

type LabelBuilder struct {
	strings *StringTable
	labels  []int32
	seen    map[string]struct{}
}

func NewLabelBuilder(strings *StringTable) *LabelBuilder {
	return &LabelBuilder{strings: strings}
}

func (lb *LabelBuilder) WithLabelSet(pairs ...string) *LabelBuilder {
	if len(pairs)%2 == 1 {
		panic("expected even number of values")
	}
	s := len(lb.labels)
	lb.labels = slices.Grow(lb.labels, len(pairs)+1)[:s+len(pairs)+1]
	lb.labels[s] = int32(len(pairs) / 2)
	for i := range pairs {
		lb.labels[s+i+1] = lb.strings.Put(pairs[i])
	}
	return lb
}

func (lb *LabelBuilder) Put(x []int32, strings []string) {
	if len(x) == 0 {
		return
	}
	if lb.seen == nil {
		lb.seen = make(map[string]struct{})
	}
	var skip int
	for i, v := range x {
		if i == skip {
			skip += int(v)*2 + 1
			continue
		}
		x[i] = lb.strings.Put(strings[v])
	}
	lb.labels = slices.Grow(lb.labels, len(x))
	pairs := LabelPairs(x)
	for pairs.Next() {
		lb.putPairs(pairs.At())
	}
}

func (lb *LabelBuilder) putPairs(p []int32) {
	if len(p) == 0 {
		return
	}
	// We only copy the labels if this is the first time we see it.
	// The fact that we assume that the order of labels is the same
	// across all datasets is a precondition, therefore, we can
	// use pairs as a key.
	k := int32string(p)
	if _, ok := lb.seen[k]; ok {
		return
	}
	lb.labels = append(lb.labels, int32(len(p)/2))
	lb.labels = append(lb.labels, p...)
	lb.seen[strings.Clone(k)] = struct{}{}
}

func (lb *LabelBuilder) Build() []int32 {
	c := make([]int32, len(lb.labels))
	copy(c, lb.labels)
	lb.labels = lb.labels[:0]
	clear(lb.seen)
	return c
}

func FindDatasets(md *metastorev1.BlockMeta, matchers ...*labels.Matcher) goiter.Seq[*metastorev1.Dataset] {
	st := NewStringTable()
	st.Import(md)
	lm := NewLabelMatcher(st.Strings, matchers)
	if !lm.IsValid() {
		return func(func(*metastorev1.Dataset) bool) {}
	}
	return func(yield func(*metastorev1.Dataset) bool) {
		for i := range md.Datasets {
			ds := md.Datasets[i]
			if !lm.Matches(ds.Labels) {
				continue
			}
			if !yield(ds) {
				return
			}
		}
	}
}

func LabelPairs(ls []int32) iter.Iterator[[]int32] { return &labelPairs{labels: ls} }

type labelPairs struct {
	labels []int32
	off    int
	len    int
}

func (p *labelPairs) Err() error   { return nil }
func (p *labelPairs) Close() error { return nil }

func (p *labelPairs) At() []int32 { return p.labels[p.off : p.off+p.len] }

func (p *labelPairs) Next() bool {
	if p.len > 0 {
		p.off += p.len
	}
	if p.off >= len(p.labels) {
		return false
	}
	p.len = int(p.labels[p.off]) * 2
	p.off++
	return p.off+p.len <= len(p.labels)
}

type LabelMatcher struct {
	eq      []matcher
	neq     []matcher
	keep    []int32
	keepStr []string

	strings []string
	checked map[string]bool
	matched int32
	nomatch bool
}

type matcher struct {
	*labels.Matcher
	name int32
}

func NewLabelMatcher(strings []string, matchers []*labels.Matcher, keep ...string) *LabelMatcher {
	s := make(map[string]int32, len(matchers)*2+len(keep))
	for _, m := range matchers {
		s[m.Name] = 0
		s[m.Value] = 0
	}
	for _, k := range keep {
		s[k] = 0
	}
	for i, x := range strings {
		if v, ok := s[x]; ok && v == 0 {
			s[x] = int32(i)
		}
	}
	lm := &LabelMatcher{
		eq:      make([]matcher, 0, len(matchers)),
		neq:     make([]matcher, 0, len(matchers)),
		keep:    make([]int32, len(keep)),
		keepStr: keep,
		checked: make(map[string]bool),
		strings: strings,
	}
	for _, m := range matchers {
		if m.Name == "" {
			continue
		}
		n := s[m.Name]
		switch m.Type {
		case labels.MatchEqual:
			if v := s[m.Value]; m.Value != "" && (n < 1 || v < 1) {
				lm.nomatch = true
				return lm
			}
			lm.eq = append(lm.eq, matcher{Matcher: m, name: n})
		case labels.MatchRegexp:
			lm.eq = append(lm.eq, matcher{Matcher: m, name: n})
		case labels.MatchNotEqual, labels.MatchNotRegexp:
			lm.neq = append(lm.neq, matcher{Matcher: m, name: n})
		}
	}
	// Find the indices of the labels to keep.
	// If the label is not found or is an empty string,
	// it will always be an empty string at the output.
	for i, k := range keep {
		lm.keep[i] = s[k]
	}
	return lm
}

func (lm *LabelMatcher) IsValid() bool { return !lm.nomatch }

// Matches reports whether the given set of labels matches the matchers.
// Note that at least one labels set must satisfy matchers to return true.
// For negations, all labels sets must satisfy the matchers to return true.
// TODO(kolesnikovae): This might be really confusing; it's worth relaxing it.
func (lm *LabelMatcher) Matches(labels []int32) bool {
	pairs := LabelPairs(labels)
	var matches bool
	for pairs.Next() {
		if lm.MatchesPairs(pairs.At()) {
			matches = true
			// If no keep labels are specified, we can return early.
			// Otherwise, we need to scan all the label sets to
			// collect matching ones.
			if len(lm.keep) == 0 {
				return true
			}
		}
	}
	return matches
}

// CollectMatches returns a new set of labels with only the labels
// that satisfy the match expressions and that are in the keep list.
func (lm *LabelMatcher) CollectMatches(dst, labels []int32) ([]int32, bool) {
	pairs := LabelPairs(labels)
	var matches bool
	for pairs.Next() {
		p := pairs.At()
		if lm.MatchesPairs(p) {
			matches = true
			// If no keep labels are specified, we can return early.
			// Otherwise, we need to scan all the label sets to
			// collect matching ones.
			if len(lm.keep) == 0 {
				return dst, true
			}
			dst = lm.strip(dst, p)
		}
	}
	return dst, matches
}

// strip returns a new length-prefixed slice of pairs
// with only the labels that are in the keep list.
func (lm *LabelMatcher) strip(dst, pairs []int32) []int32 {
	// Length-prefix stub: we only know it after we iterate
	// over the pairs.
	s := len(dst)
	c := len(lm.keep) * 2
	dst = slices.Grow(dst, c+1)
	dst = append(dst, 0)
	var m int32
	for _, n := range lm.keep {
		if n < 1 {
			// Ignore not found labels.
			continue
		}
		for k := 0; k < len(pairs); k += 2 {
			if pairs[k] == n {
				dst = append(dst, pairs[k], pairs[k+1])
				m++
				break
			}
		}
	}
	// Write the actual number of pairs as a prefix.
	dst[s] = m
	return dst
}

func (lm *LabelMatcher) MatchesPairs(pairs []int32) bool {
	k := int32string(pairs)
	m, found := lm.checked[k]
	if !found {
		m = lm.checkMatches(pairs)
		lm.checked[strings.Clone(k)] = m
		if m {
			lm.matched++
		}
	}
	return m
}

func (lm *LabelMatcher) checkMatches(pairs []int32) bool {
	if len(pairs)%2 == 1 {
		// Invalid pairs.
		return false
	}
	for _, m := range lm.eq {
		var matches bool
		for k := 0; k < len(pairs); k += 2 {
			if pairs[k] != m.name {
				continue
			}
			v := lm.strings[pairs[k+1]]
			matches = m.Matches(v)
			break
		}
		if !matches {
			return false
		}
	}
	// At this point, we know that all eq matchers have matched.
	for _, m := range lm.neq {
		for k := 0; k < len(pairs); k += 2 {
			if pairs[k] != m.name {
				continue
			}
			v := lm.strings[pairs[k+1]]
			if !m.Matches(v) {
				return false
			}
			break
		}
	}
	return true
}

type LabelsCollector struct {
	strings *StringTable
	dict    map[string]struct{}
	tmp     []int32
	keys    []int32
}

func NewLabelsCollector(labels ...string) *LabelsCollector {
	s := &LabelsCollector{
		dict:    make(map[string]struct{}),
		strings: NewStringTable(),
	}
	s.keys = make([]int32, len(labels))
	s.tmp = make([]int32, len(labels))
	for i, k := range labels {
		s.keys[i] = s.strings.Put(k)
	}
	return s
}

// CollectMatches from the given matcher.
//
// The matcher and collect MUST be configured to keep the same
// set of labels, in the exact order.
//
// A single collector may collect labels from multiple matchers.
func (s *LabelsCollector) CollectMatches(lm *LabelMatcher) {
	if len(lm.keep) == 0 || lm.nomatch || len(lm.checked) == 0 {
		return
	}
	for set, match := range lm.checked {
		if !match {
			continue
		}
		// Project values of the keep labels to tmp,
		// and resolve their strings.
		clear(s.tmp)
		p := int32s(set)
		// Note that we're using the matcher's keep labels
		// and not local 'keys'.
		for i, n := range lm.keep {
			for k := 0; k < len(p); k += 2 {
				if p[k] == n {
					s.tmp[i] = p[k+1]
					break
				}
			}
		}
		for i := range s.tmp {
			s.tmp[i] = s.strings.Put(lm.strings[s.tmp[i]])
		}
		// Check if we already saw the label set.
		x := int32string(s.tmp)
		if _, ok := s.dict[x]; ok {
			continue
		}
		s.dict[strings.Clone(x)] = struct{}{}
	}
}

func (s *LabelsCollector) Unique() goiter.Seq[*typesv1.Labels] {
	return func(yield func(*typesv1.Labels) bool) {
		for k := range s.dict {
			l := &typesv1.Labels{Labels: make([]*typesv1.LabelPair, len(s.keys))}
			for i, v := range int32s(k) {
				l.Labels[i] = &typesv1.LabelPair{
					Name:  s.strings.Strings[s.keys[i]],
					Value: s.strings.Strings[v],
				}
			}
			if !yield(l) {
				return
			}
		}
	}
}

func int32string(data []int32) string {
	if len(data) == 0 {
		return ""
	}
	return unsafe.String((*byte)(unsafe.Pointer(&data[0])), len(data)*4)
}

func int32s(s string) []int32 {
	if len(s) == 0 {
		return nil
	}
	return unsafe.Slice((*int32)(unsafe.Pointer(unsafe.StringData(s))), len(s)/4)
}
