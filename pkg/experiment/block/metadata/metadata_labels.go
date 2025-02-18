package metadata

import (
	"slices"
	"strings"
	"unsafe"

	"github.com/prometheus/prometheus/model/labels"
	"golang.org/x/exp/maps"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/model"
)

// TODO(kolesnikovae): LabelBuilder pool.

const (
	LabelNameTenantDataset     = "__tenant_dataset__"
	LabelValueDatasetTSDBIndex = "dataset_tsdb_index"
)

type LabelBuilder struct {
	strings  *StringTable
	labels   []int32
	constant []int32
	keys     []int32
	seen     map[string]struct{}
}

func NewLabelBuilder(strings *StringTable) *LabelBuilder {
	return &LabelBuilder{strings: strings}
}

func (lb *LabelBuilder) WithConstantPairs(pairs ...string) *LabelBuilder {
	if len(pairs)%2 == 1 {
		return lb
	}
	lb.constant = slices.Grow(lb.constant[:0], len(pairs))[:len(pairs)]
	for i := 0; i < len(pairs); i++ {
		lb.constant[i] = lb.strings.Put(pairs[i])
	}
	return lb
}

func (lb *LabelBuilder) WithLabelNames(names ...string) *LabelBuilder {
	lb.keys = slices.Grow(lb.keys[:0], len(names))[:len(names)]
	for i, n := range names {
		lb.keys[i] = lb.strings.Put(n)
	}
	return lb
}

func (lb *LabelBuilder) CreateLabels(values ...string) bool {
	if len(values) != len(lb.keys) {
		return false
	}
	// We're going to add the length of pairs, the constant pairs,
	// and then the variadic key-value pairs: p pairs total.
	p := len(lb.constant)/2 + len(lb.keys)
	n := 1 + p*2 // n elems total.
	lb.labels = slices.Grow(lb.labels, n)
	lb.labels = append(lb.labels, int32(p))
	lb.labels = append(lb.labels, lb.constant...)
	for i := 0; i < len(values); i++ {
		lb.labels = append(lb.labels, lb.keys[i], lb.strings.Put(values[i]))
	}
	return true
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
	return c
}

func (lb *LabelBuilder) BuildPairs(pairs ...string) []int32 {
	lb.WithConstantPairs(pairs...)
	lb.CreateLabels()
	return lb.Build()
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

func NewLabelMatcher(strings *StringTable, matchers []*labels.Matcher, keep ...string) *LabelMatcher {
	lm := &LabelMatcher{
		eq:      make([]matcher, 0, len(matchers)),
		neq:     make([]matcher, 0, len(matchers)),
		keep:    make([]int32, len(keep)),
		keepStr: keep,
		checked: make(map[string]bool),
		strings: strings.Strings,
	}
	for _, m := range matchers {
		n := strings.LookupString(m.Name)
		if m.Type == labels.MatchEqual || m.Type == labels.MatchRegexp {
			if n < 1 {
				// No matches are possible if a label is not found
				// in the string table or is an empty string (0).
				lm.nomatch = true
				return lm
			}
			lm.eq = append(lm.eq, matcher{Matcher: m, name: n})
		} else {
			lm.neq = append(lm.neq, matcher{Matcher: m, name: n})
		}
	}
	// Find the indices of the labels to keep.
	// If the label is not found or is an empty string,
	// it will always be an empty string at the output.
	for i, k := range keep {
		lm.keep[i] = strings.LookupString(k)
	}
	return lm
}

func (lm *LabelMatcher) IsValid() bool { return !lm.nomatch }

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

func (lm *LabelMatcher) AllMatches() []model.Labels {
	if len(lm.keep) == 0 || lm.nomatch || len(lm.checked) == 0 {
		return nil
	}
	matched := make(map[string]model.Labels, lm.matched)
	for k, match := range lm.checked {
		if match {
			values := lm.values(int32s(k))
			if _, found := matched[int32string(values)]; !found {
				matched[strings.Clone(int32string(values))] = lm.labels(values)
			}
		}
	}
	return maps.Values(matched)
}

func (lm *LabelMatcher) values(pairs []int32) []int32 {
	values := make([]int32, len(lm.keep))
	for i, n := range lm.keep {
		if n < 1 {
			// Skip invalid keep labels.
			continue
		}
		for k := 0; k < len(pairs); k += 2 {
			if pairs[k] == n {
				values[i] = pairs[k+1]
				break
			}
		}
	}
	return values
}

func (lm *LabelMatcher) labels(values []int32) model.Labels {
	ls := make(model.Labels, len(values))
	for i, v := range values {
		ls[i] = &typesv1.LabelPair{
			Name:  lm.keepStr[i],
			Value: lm.strings[v],
		}
	}
	return ls
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
