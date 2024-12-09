package block

import (
	"slices"
	"unsafe"

	"github.com/prometheus/prometheus/model/labels"
	"golang.org/x/exp/maps"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/model"
)

type LabelBuilder struct {
	strings  *MetadataStrings
	labels   []int32
	constant []int32
	keys     []int32
}

func NewLabelBuilder(strings *MetadataStrings) *LabelBuilder {
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

func (lb *LabelBuilder) Build() []int32 {
	c := make([]int32, len(lb.labels))
	copy(c, lb.labels)
	lb.labels = lb.labels[:0]
	return c
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

func NewLabelMatcher(strings *MetadataStrings, matchers []*labels.Matcher, keep ...string) *LabelMatcher {
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

func (lm *LabelMatcher) Matches(pairs []int32) bool {
	k := *(*string)(unsafe.Pointer(&pairs))
	m, found := lm.checked[k]
	if !found {
		m = lm.checkMatches(pairs)
		// Copy the key.
		lm.checked[string(pairs)] = m
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

func (lm *LabelMatcher) Matched() []model.Labels {
	if len(lm.keep) == 0 || lm.nomatch || len(lm.checked) == 0 {
		return nil
	}
	matched := make(map[string]model.Labels, lm.matched)
	for k, match := range lm.checked {
		if match {
			values := lm.values(k)
			if _, found := matched[values]; !found {
				matched[values] = lm.labels([]int32(values))
			}
		}
	}
	return maps.Values(matched)
}

func (lm *LabelMatcher) values(pairs string) string {
	p := []int32(pairs)
	values := make([]int32, len(lm.keep))
	for i, n := range lm.keep {
		if n < 1 {
			// Skip invalid keep labels.
			continue
		}
		for k := 0; k < len(pairs); k += 2 {
			if p[k] == n {
				values[i] = p[k+1]
				break
			}
		}
	}
	return string(values)
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
