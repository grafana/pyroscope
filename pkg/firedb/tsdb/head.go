package index

import (
	"context"
	"sort"
	"sync"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"go.uber.org/atomic"

	"github.com/grafana/fire/pkg/firedb/tsdb/index"
	firemodel "github.com/grafana/fire/pkg/model"
)

type profileLabels struct {
	lbs        firemodel.Labels
	fp         model.Fingerprint
	mint, maxt *atomic.Int64
}

type Head struct {
	ix                  *BitPrefixInvertedIndex
	profilesLabels      sync.Map
	totalProfilesLabels atomic.Int64
}

func NewHead(totalShards uint32) (*Head, error) {
	ix, err := NewBitPrefixWithShards(totalShards)
	if err != nil {
		return nil, err
	}
	return &Head{
		ix: ix,
	}, nil
}

// Add adds a new profile labels to the index for the given timestamp in nanoseconds.
// lbs is expected to be sorted.
func (h *Head) Add(ts int64, lbs firemodel.Labels) model.Fingerprint {
	// todo handle collisions
	fp := model.Fingerprint(lbs.Hash())
	pl, loaded := h.profilesLabels.LoadOrStore(fp, &profileLabels{
		lbs:  lbs,
		mint: atomic.NewInt64(ts),
		maxt: atomic.NewInt64(ts),
		fp:   fp,
	})
	if loaded {
		pl := pl.(*profileLabels)
		for {
			if min := pl.mint.Load(); ts < min {
				if pl.mint.CAS(min, ts) {
					break
				}
				continue
			}
			break
		}
		for {
			if max := pl.maxt.Load(); ts > max {
				if pl.maxt.CAS(max, ts) {
					break
				}
				continue
			}
			break
		}

		return fp
	}
	h.ix.Add(lbs, fp)
	h.totalProfilesLabels.Inc()
	return fp
}

// ForMatchingProfiles iterates through all matching profiles and calls f for each profiles label set.
func (h *Head) ForMatchingProfiles(matchers []*labels.Matcher, fn func(lbs firemodel.Labels, fp model.Fingerprint, mint int64, maxt int64) error) error {
	filters, matchers := SplitFiltersAndMatchers(matchers)
	ids, err := h.ix.Lookup(matchers, nil)
	if err != nil {
		return err
	}

outer:
	for _, fp := range ids {
		value, ok := h.profilesLabels.Load(fp)
		if !ok {
			// If a profile labels is missing here, it has already been flushed
			// and is supposed to be picked up from storage by querier
			continue
		}
		profile := value.(*profileLabels)
		for _, filter := range filters {
			if !filter.Matches(profile.lbs.Get(filter.Name)) {
				continue outer
			}
		}

		err := fn(profile.lbs, profile.fp, profile.mint.Load(), profile.maxt.Load())
		if err != nil {
			return err
		}
	}
	return nil
}

func (h *Head) LabelNames() ([]string, error) {
	return h.ix.LabelNames(nil)
}

func (h *Head) LabelValues(name string) ([]string, error) {
	return h.ix.LabelValues(name, nil)
}

func (h *Head) WriteTo(ctx context.Context, path string) error {
	writer, err := index.NewWriter(ctx, path)
	if err != nil {
		return err
	}
	pfs := make([]*profileLabels, 0, h.totalProfilesLabels.Load())
	h.profilesLabels.Range(func(key, value interface{}) bool {
		pfs = append(pfs, value.(*profileLabels))
		return true
	})
	// sort by fp
	sort.Slice(pfs, func(i, j int) bool {
		if pfs[i].fp != pfs[j].fp {
			return pfs[i].fp < pfs[j].fp
		}
		return firemodel.CompareLabelPairs(pfs[i].lbs, pfs[j].lbs) < 0
	})

	symbolsMap := make(map[string]struct{})
	for _, s := range pfs {
		for _, l := range s.lbs {
			symbolsMap[l.Name] = struct{}{}
			symbolsMap[l.Value] = struct{}{}
		}
	}

	// Sort symbols
	symbols := make([]string, 0, len(symbolsMap))
	for s := range symbolsMap {
		symbols = append(symbols, s)
	}
	sort.Strings(symbols)

	// Add symbols
	for _, symbol := range symbols {
		if err := writer.AddSymbol(symbol); err != nil {
			return err
		}
	}
	// Add series
	for i, s := range pfs {
		if err := writer.AddSeries(storage.SeriesRef(i), s.lbs, s.fp, index.ChunkMeta{
			MinTime: s.mint.Load(),
			MaxTime: s.maxt.Load(),
		}); err != nil {
			return err
		}
	}

	return writer.Close()
}

// SplitFiltersAndMatchers splits empty matchers off, which are treated as filters, see #220
func SplitFiltersAndMatchers(allMatchers []*labels.Matcher) (filters, matchers []*labels.Matcher) {
	for _, matcher := range allMatchers {
		// If a matcher matches "", we need to fetch possible chunks where
		// there is no value and will therefore not be in our label index.
		// e.g. {foo=""} and {foo!="bar"} both match "", so we need to return
		// chunks which do not have a foo label set. When looking entries in
		// the index, we should ignore this matcher to fetch all possible chunks
		// and then filter on the matcher after the chunks have been fetched.
		if matcher.Matches("") {
			filters = append(filters, matcher)
		} else {
			matchers = append(matchers, matcher)
		}
	}
	return
}
