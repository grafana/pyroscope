package exporter

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
)

// matchedLabels returns map of KV pairs from the given key that match
// tag matchers keys of the rule regardless of their values, e.g.:
//   key:     app{foo=bar,baz=qux}
//   query:   app{foo="xxx"}
//   matched: {__name__: app, foo: bar}
//
// N.B: application name label is always first.
//
// The key must include labels required by the rule expression, otherwise
// the function returns empty labels and false.
func (r *rule) matchedLabels(key *segment.Key) (matchedLabels, bool) {
	appName := key.AppName()
	if appName != r.qry.AppName {
		return nil, false
	}
	// This is required for a case when there are no tag matchers.
	z := matchedLabels{{flameql.ReservedTagKeyName, appName}}
	l := key.Labels()
	// Matchers may refer the same labels, duplicates should be removed.
	set := map[string]struct{}{}
	for _, m := range r.qry.Matchers {
		v, ok := l[m.Key]
		if !ok {
			// If the matcher label is required (e.g. the matcher
			// operator is EQL or EQL_REGEX) but not present, return.
			if m.IsNegation() {
				continue
			}
			return nil, false
		}
		if _, ok = set[m.Key]; !ok {
			// Note that Matchers are sorted.
			z = append(z, label{m.Key, v})
			set[m.Key] = struct{}{}
		}
	}
	return z, true
}

// matchedLabels contain KV pairs from a dimension key that match
// tag matchers keys of a rule regardless of their values.
type matchedLabels []label

type label struct{ key, value string }

// hash returns FNV-1a hash of labels key value pairs.
func (m matchedLabels) hash() uint64 {
	h := fnv.New64a()
	for k, v := range m {
		_, _ = fmt.Fprint(h, k, ":", v, ";")
	}
	return binary.BigEndian.Uint64(h.Sum(nil))
}

func (m matchedLabels) labels() prometheus.Labels {
	p := make(prometheus.Labels, len(m))
	for _, l := range m {
		p[l.key] = l.value
	}
	return p
}
