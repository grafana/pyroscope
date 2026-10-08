package main

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/symdb"
)

// An empty salt disables anonymization. The salt is never written to the dump.
type replayAnonymizer string

func (a replayAnonymizer) hash(s string) string {
	sum := sha256.Sum256([]byte(string(a) + s))
	return hex.EncodeToString(sum[:])
}

func replayProfileTypeLabel(name string) bool {
	switch name {
	case phlaremodel.LabelNameProfileType, phlaremodel.LabelNameProfileName,
		phlaremodel.LabelNameType, phlaremodel.LabelNameUnit,
		phlaremodel.LabelNamePeriodType, phlaremodel.LabelNamePeriodUnit:
		return true
	default:
		return false
	}
}

func (a replayAnonymizer) labels(ls phlaremodel.Labels) phlaremodel.Labels {
	if a == "" {
		return ls
	}
	out := make(phlaremodel.Labels, len(ls))
	for i, l := range ls {
		name, value := l.Name, l.Value
		if !replayProfileTypeLabel(name) {
			value = a.hash(value)
			if name != phlaremodel.LabelNameServiceName {
				// Hex digests may start with a digit, which is not a valid first
				// character in a legacy Prometheus label name.
				name = "_" + a.hash(name)
			}
		}
		out[i] = &typesv1.LabelPair{Name: name, Value: value}
	}
	sort.Sort(out)
	return out
}

type anonymizedReplayPartition struct {
	symdb.PartitionReader
	symbols *symdb.Symbols
}

func (p anonymizedReplayPartition) Symbols() *symdb.Symbols { return p.symbols }

func (a replayAnonymizer) partition(p symdb.PartitionReader) symdb.PartitionReader {
	if a == "" {
		return p
	}
	// Copy the string table, not the shared reader's storage. All indexes and
	// trees stay unchanged. Hash once per partition, not once per profile.
	symbols := *p.Symbols()
	strings := make([]string, len(symbols.Strings))
	for i, s := range symbols.Strings {
		// pprof requires the empty string sentinel to stay empty.
		if s != "" {
			strings[i] = a.hash(s)
		}
	}
	symbols.Strings = strings
	return anonymizedReplayPartition{PartitionReader: p, symbols: &symbols}
}
