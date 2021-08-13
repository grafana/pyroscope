package exporter

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"

	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
)

// matchLabelNames returns map of KV pairs from the given key that match
// tag matchers keys of the rule regardless of their values, e.g.:
//   key:     app{foo=bar,baz=qux}
//   query:   app{foo="xxx"}
//   matched: {__name__: app, foo: bar}
//
// The key must include labels required by the rule expression, otherwise
// the function returns empty labels and false.
func (r *rule) matchLabelNames(key *segment.Key) (labels, bool) {
	appName := key.AppName()
	if appName != r.qry.AppName {
		return nil, false
	}
	// This is required for a case when there are no tag matchers.
	z := labels{{flameql.ReservedTagKeyName, appName}}
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

// labels contain KV label pairs from a segment key.
type labels []label

type label struct{ key, value string }

func (l labels) Len() int           { return len(l) }
func (l labels) Less(i, j int) bool { return l[i].key < l[j].key }
func (l labels) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }

// hash returns FNV-1a hash of labels key value pairs.
func (l labels) hash() uint64 {
	h := fnv.New64a()
	for k, v := range l {
		_, _ = fmt.Fprint(h, k, ":", v, ";")
	}
	return binary.BigEndian.Uint64(h.Sum(nil))
}
