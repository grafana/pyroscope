package firedb

import (
	"context"
	"sort"
	"sync"

	"github.com/google/uuid"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"go.uber.org/atomic"

	schemav1 "github.com/grafana/fire/pkg/firedb/schemas/v1"
	"github.com/grafana/fire/pkg/firedb/tsdb"
	"github.com/grafana/fire/pkg/firedb/tsdb/index"
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
}

func newProfileIndex(totalShards uint32) (*profilesIndex, error) {
	ix, err := tsdb.NewBitPrefixWithShards(totalShards)
	if err != nil {
		return nil, err
	}
	return &profilesIndex{
		ix:            ix,
		profilesPerFP: make(map[model.Fingerprint]*profileLabels),
		totalProfiles: atomic.NewInt64(0),
	}, nil
}

// Add a new set of profile to the index.
// The seriesRef are expected to match the profile labels passed in.
func (pi *profilesIndex) Add(ps *schemav1.Profile, lbs []firemodel.Labels) {
	pi.mutex.Lock()
	defer pi.mutex.Unlock()
	for i, fp := range ps.SeriesRefs {
		profiles, ok := pi.profilesPerFP[fp]
		if !ok {
			lbs := pi.ix.Add(lbs[i], fp)
			profiles = &profileLabels{
				lbs:      lbs,
				fp:       fp,
				profiles: []*schemav1.Profile{ps},
			}
			pi.profilesPerFP[fp] = profiles
			continue
		}
		profiles.profiles = append(profiles.profiles, ps)
	}
	pi.totalProfiles.Inc()
}

// forMatchingProfiles iterates through all matching profiles and calls f for each profiles.
// The profile contains multiple samples not all of them are matching the matchers.
// You can use the fingerprint to filter the samples by his position in the returned profile.
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
			if err := fn(profile.lbs, profile.fp, p); err != nil {
				return err
			}
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

// writeTo writes the profiles tsdb index to the specified filepath.
func (pi *profilesIndex) writeTo(ctx context.Context, path string) error {
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
		min, max := minmax(s.profiles)
		if err := writer.AddSeries(storage.SeriesRef(i), s.lbs, s.fp, index.ChunkMeta{
			MinTime: min,
			MaxTime: max,
		}); err != nil {
			return err
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

type profilesHelper struct{}

func (*profilesHelper) key(s *schemav1.Profile) profilesKey {
	id := s.ID
	if id == uuid.Nil {
		id = uuid.New()
	}
	return profilesKey{
		ID: id,
	}
}

func (*profilesHelper) addToRewriter(r *rewriter, elemRewriter idConversionTable) {
	r.locations = elemRewriter
}

func (*profilesHelper) rewrite(r *rewriter, s *schemav1.Profile) error {
	for pos := range s.Comment {
		r.strings.rewrite(&s.Comment[pos])
	}

	r.strings.rewrite(&s.DropFrames)
	r.strings.rewrite(&s.KeepFrames)

	return nil
}

type profilesKey struct {
	ID uuid.UUID
}
