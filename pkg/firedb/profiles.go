package firedb

import (
	"context"
	"sort"
	"sync"
	"unsafe"

	"github.com/google/uuid"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"go.uber.org/atomic"

	schemav1 "github.com/grafana/fire/pkg/firedb/schemas/v1"
	"github.com/grafana/fire/pkg/firedb/tsdb"
	"github.com/grafana/fire/pkg/firedb/tsdb/index"
	"github.com/grafana/fire/pkg/iter"
	firemodel "github.com/grafana/fire/pkg/model"
)

type profileLabels struct {
	lbs      firemodel.Labels
	fp       model.Fingerprint
	profiles []*schemav1.Profile
}

type profilesIndex struct {
	ix *tsdb.BitPrefixInvertedIndex
	// todo: like the inverted index we might want to shard fingerprint to avoid contentions.
	profilesPerFP map[model.Fingerprint]*profileLabels
	mutex         sync.RWMutex
	totalProfiles *atomic.Int64
	totalSeries   *atomic.Int64

	metrics *headMetrics
}

func newProfileIndex(totalShards uint32, metrics *headMetrics) (*profilesIndex, error) {
	ix, err := tsdb.NewBitPrefixWithShards(totalShards)
	if err != nil {
		return nil, err
	}
	return &profilesIndex{
		ix:            ix,
		profilesPerFP: make(map[model.Fingerprint]*profileLabels),
		totalProfiles: atomic.NewInt64(0),
		totalSeries:   atomic.NewInt64(0),
		metrics:       metrics,
	}, nil
}

// Add a new set of profile to the index.
// The seriesRef are expected to match the profile labels passed in.
func (pi *profilesIndex) Add(ps *schemav1.Profile, lbs firemodel.Labels, profileName string) {
	pi.mutex.Lock()
	defer pi.mutex.Unlock()
	profiles, ok := pi.profilesPerFP[ps.SeriesFingerprint]
	if !ok {
		lbs := pi.ix.Add(lbs, ps.SeriesFingerprint)
		profiles = &profileLabels{
			lbs: lbs,
			fp:  ps.SeriesFingerprint,
		}
		pi.profilesPerFP[ps.SeriesFingerprint] = profiles
		pi.totalSeries.Inc()
		pi.metrics.seriesCreated.WithLabelValues(profileName).Inc()
	}
	profiles.profiles = append(profiles.profiles, ps)
	pi.totalProfiles.Inc()
	pi.metrics.profilesCreated.WithLabelValues(profileName).Inc()
}

// forMatchingProfiles iterates through all matching profiles and calls f for each profiles.
// The profile contains multiple samples not all of them are matching the matchers.
// You can use sampleIdx to filter the samples by his position in the returned profile.
// The returned profile is not sorted.
func (pi *profilesIndex) forMatchingProfiles(matchers []*labels.Matcher,
	fn func(lbs firemodel.Labels, fp model.Fingerprint, profile *schemav1.Profile) error,
) error {
	filters, matchers := SplitFiltersAndMatchers(matchers)
	ids, err := pi.ix.Lookup(matchers, nil)
	if err != nil {
		return err
	}

	pi.mutex.RLock()
	defer pi.mutex.RUnlock()

outer:
	for _, fp := range ids {
		profile, ok := pi.profilesPerFP[fp]
		if !ok {
			// If a profile labels is missing here, it has already been flushed
			// and is supposed to be picked up from storage by querier
			continue
		}
		for _, filter := range filters {
			if !filter.Matches(profile.lbs.Get(filter.Name)) {
				continue outer
			}
		}
		for _, p := range profile.profiles {
			if p.SeriesFingerprint == fp {
				if err := fn(profile.lbs, profile.fp, p); err != nil {
					return err
				}
			}
		}

	}
	return nil
}

func (pi *profilesIndex) SelectProfiles(matchers []*labels.Matcher, start, end model.Time) (iter.Iterator[Profile], error) {
	filters, matchers := SplitFiltersAndMatchers(matchers)
	ids, err := pi.ix.Lookup(matchers, nil)
	if err != nil {
		return nil, err
	}

	pi.mutex.RLock()
	defer pi.mutex.RUnlock()

	iters := make([]iter.Iterator[Profile], 0, len(ids))
outer:
	for _, fp := range ids {
		profile, ok := pi.profilesPerFP[fp]
		if !ok {
			// If a profile labels is missing here, it has already been flushed
			// and is supposed to be picked up from storage by querier
			continue
		}
		for _, filter := range filters {
			if !filter.Matches(profile.lbs.Get(filter.Name)) {
				continue outer
			}
		}
		iters = append(iters,
			NewSeriesIterator(
				profile.lbs,
				profile.fp,
				iter.NewTimeRangedIterator(iter.NewSliceIterator(profile.profiles), start, end),
			),
		)
	}
	return iter.NewSortProfileIterator(iters), nil
}

type ProfileWithLabels struct {
	*schemav1.Profile
	lbs firemodel.Labels
	fp  model.Fingerprint
}

func (p ProfileWithLabels) Timestamp() model.Time {
	return model.TimeFromUnixNano(p.Profile.TimeNanos)
}

func (p ProfileWithLabels) Fingerprint() model.Fingerprint {
	return p.fp
}

func (p ProfileWithLabels) Labels() firemodel.Labels {
	return p.lbs
}

type SeriesIterator struct {
	iter.Iterator[*schemav1.Profile]
	curr ProfileWithLabels
	fp   model.Fingerprint
	lbs  firemodel.Labels
}

func NewSeriesIterator(labels firemodel.Labels, fingerprint model.Fingerprint, it iter.Iterator[*schemav1.Profile]) *SeriesIterator {
	return &SeriesIterator{
		Iterator: it,
		fp:       fingerprint,
		lbs:      labels,
	}
}

func (it *SeriesIterator) Next() bool {
	if !it.Iterator.Next() {
		return false
	}
	it.curr = ProfileWithLabels{
		Profile: it.Iterator.At(),
		lbs:     it.lbs,
		fp:      it.fp,
	}
	return true
}

func (it *SeriesIterator) At() Profile {
	return it.curr
}

// forMatchingLabels iterates through all matching label sets and calls f for each labels set.
func (pi *profilesIndex) forMatchingLabels(matchers []*labels.Matcher,
	fn func(lbs firemodel.Labels, fp model.Fingerprint) error,
) error {
	filters, matchers := SplitFiltersAndMatchers(matchers)
	ids, err := pi.ix.Lookup(matchers, nil)
	if err != nil {
		return err
	}

	pi.mutex.RLock()
	defer pi.mutex.RUnlock()

outer:
	for _, fp := range ids {
		profile, ok := pi.profilesPerFP[fp]
		if !ok {
			// If a profile labels is missing here, it has already been flushed
			// and is supposed to be picked up from storage by querier
			continue
		}
		for _, filter := range filters {
			if !filter.Matches(profile.lbs.Get(filter.Name)) {
				continue outer
			}
		}
		if err := fn(profile.lbs, fp); err != nil {
			return err
		}
	}
	return nil
}

func (pi *profilesIndex) allProfiles() []*schemav1.Profile {
	total := pi.totalProfiles.Load()
	result := make([]*schemav1.Profile, 0, total)
	uniq := make(map[uuid.UUID]struct{}, total)

	pi.mutex.RLock()
	defer pi.mutex.RUnlock()

	for _, profile := range pi.profilesPerFP {
		for _, p := range profile.profiles {
			if _, ok := uniq[p.ID]; !ok {
				uniq[p.ID] = struct{}{}
				result = append(result, p)
			}
		}
	}

	return result
}

// WriteTo writes the profiles tsdb index to the specified filepath.
func (pi *profilesIndex) WriteTo(ctx context.Context, path string) error {
	writer, err := index.NewWriter(ctx, path)
	if err != nil {
		return err
	}
	pi.mutex.RLock()
	defer pi.mutex.RUnlock()

	pfs := make([]*profileLabels, 0, len(pi.profilesPerFP))

	for _, p := range pi.profilesPerFP {
		pfs = append(pfs, p)
	}

	// sort by fp
	sort.Slice(pfs, func(i, j int) bool {
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
		min, max := minmax(s.profiles)
		if err := writer.AddSeries(storage.SeriesRef(i), s.lbs, s.fp, index.ChunkMeta{
			MinTime: min,
			MaxTime: max,
			// We store the series Index from the head with the series to use when retrieving data from parquet.
			SeriesIndex: uint32(i),
		}); err != nil {
			return err
		}
		// also rewrite the SeriesIndex
		for _, p := range s.profiles {
			if p.SeriesFingerprint == s.fp {
				p.SeriesIndex = uint32(i)
			}
		}
	}

	return writer.Close()
}

func minmax(profiles []*schemav1.Profile) (int64, int64) {
	var min, max int64
	switch len(profiles) {
	case 0:
		return 0, 0
	case 1:
		return profiles[0].TimeNanos, profiles[0].TimeNanos
	default:
		min = profiles[0].TimeNanos
		max = profiles[0].TimeNanos
		for _, p := range profiles[1:] {
			if p.TimeNanos < min {
				min = p.TimeNanos
			}
			if p.TimeNanos > max {
				max = p.TimeNanos
			}
		}
		return min, max
	}
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

const (
	profileSize = uint64(unsafe.Sizeof(schemav1.Profile{}))
	sampleSize  = uint64(unsafe.Sizeof(schemav1.Sample{}))
)

type profilesHelper struct{}

func (*profilesHelper) key(s *schemav1.Profile) noKey {
	return noKey{}
}

func (*profilesHelper) addToRewriter(r *rewriter, elemRewriter idConversionTable) {
	r.locations = elemRewriter
}

func (*profilesHelper) rewrite(r *rewriter, s *schemav1.Profile) error {
	for pos := range s.Comments {
		r.strings.rewrite(&s.Comments[pos])
	}

	r.strings.rewrite(&s.DropFrames)
	r.strings.rewrite(&s.KeepFrames)

	return nil
}

func (*profilesHelper) setID(oldID, newID uint64, p *schemav1.Profile) uint64 {
	return oldID
}

func sizeOfSample(s *schemav1.Sample) uint64 {
	return sampleSize + 8
}

func (*profilesHelper) size(p *schemav1.Profile) uint64 {
	size := profileSize

	size += 8
	size += uint64(len(p.Comments) * 8)

	for _, s := range p.Samples {
		size += sizeOfSample(s)
	}

	return size
}

func (*profilesHelper) clone(p *schemav1.Profile) *schemav1.Profile {
	return p
}

type noKey struct{}

func isNoKey(a interface{}) bool {
	_, ok := a.(noKey)
	return ok
}
