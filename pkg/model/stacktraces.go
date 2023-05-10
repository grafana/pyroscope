package model

import (
	"encoding/binary"
	"sort"
	"strings"

	"github.com/cespare/xxhash/v2"

	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
)

func MergeBatchMergeStacktraces(responses ...*ingestv1.MergeProfilesStacktracesResult) *ingestv1.MergeProfilesStacktracesResult {
	var (
		result      *ingestv1.MergeProfilesStacktracesResult
		posByName   map[string]int32
		hasher      StacktracesHasher
		stacktraces = map[uint64]*ingestv1.StacktraceSample{}
	)

	largestNames := 0

	for _, resp := range responses {
		if resp != nil {
			if len(resp.FunctionNames) > largestNames {
				largestNames = len(resp.FunctionNames)
			}
		}
	}
	rewrite := make([]int32, largestNames)

	for _, resp := range responses {
		// skip empty results
		if resp == nil || len(resp.Stacktraces) == 0 {
			continue
		}

		// first non-empty result result
		if result == nil {
			result = resp
			for _, s := range result.Stacktraces {
				stacktraces[hasher.Hashes(s.FunctionIds)] = s
			}
			continue
		}

		// build up the lookup map the first time
		if posByName == nil {
			posByName = make(map[string]int32, len(result.FunctionNames))
			for idx, n := range result.FunctionNames {
				posByName[n] = int32(idx)
			}
		}

		// lookup and add missing functionNames
		var (
			rewrite = rewrite[:len(resp.FunctionNames)]
			ok      bool
		)
		for idx, n := range resp.FunctionNames {
			rewrite[idx], ok = posByName[n]
			if ok {
				continue
			}

			// need to add functionName to list
			rewrite[idx] = int32(len(result.FunctionNames))
			result.FunctionNames = append(result.FunctionNames, n)
		}

		// rewrite existing function ids, by building a list of unique slices
		functionIDsUniq := make(map[*int32][]int32)
		for _, sample := range resp.Stacktraces {
			if len(sample.FunctionIds) == 0 {
				continue
			}
			functionIDsUniq[&sample.FunctionIds[0]] = sample.FunctionIds

		}
		// now rewrite those ids in slices
		for _, slice := range functionIDsUniq {
			for idx, functionID := range slice {
				slice[idx] = rewrite[functionID]
			}
		}
		// if the stacktraces is missing add it or merge it.
		for _, sample := range resp.Stacktraces {
			if len(sample.FunctionIds) == 0 {
				continue
			}
			hash := hasher.Hashes(sample.FunctionIds)
			if existing, ok := stacktraces[hash]; ok {
				existing.Value += sample.Value
			} else {
				stacktraces[hash] = sample
				result.Stacktraces = append(result.Stacktraces, sample)
			}
		}
	}

	// ensure nil will always be the empty response
	if result == nil {
		result = &ingestv1.MergeProfilesStacktracesResult{}
	}

	// sort stacktraces by function name
	sortStacktraces(result)

	return result
}

type StacktracesHasher struct {
	hash *xxhash.Digest
	b    [4]byte
}

// todo we might want to reuse the results to avoid allocations
func (h StacktracesHasher) Hashes(fnIds []int32) uint64 {
	if h.hash == nil {
		h.hash = xxhash.New()
	} else {
		h.hash.Reset()
	}

	for _, locID := range fnIds {
		binary.LittleEndian.PutUint32(h.b[:], uint32(locID))
		if _, err := h.hash.Write(h.b[:]); err != nil {
			panic("unable to write hash")
		}
	}
	return h.hash.Sum64()
}

// sortStacktraces sorts the stacktraces by function name
func sortStacktraces(r *ingestv1.MergeProfilesStacktracesResult) {
	sort.Slice(r.Stacktraces, func(i, j int) bool {
		pos := 0
		for {
			// check slice lengths
			if pos >= len(r.Stacktraces[i].FunctionIds) {
				break
			}
			if pos >= len(r.Stacktraces[j].FunctionIds) {
				return false
			}

			if diff := strings.Compare(r.FunctionNames[r.Stacktraces[i].FunctionIds[pos]], r.FunctionNames[r.Stacktraces[j].FunctionIds[pos]]); diff < 0 {
				break
			} else if diff > 0 {
				return false
			}
			pos++
		}

		// when we get here, i is less than j
		return true
	})
}
