package phlaredb

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"unsafe"

	"github.com/gogo/status"
	"github.com/google/uuid"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/segmentio/parquet-go"
	"go.uber.org/atomic"
	"google.golang.org/grpc/codes"

	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	"github.com/grafana/phlare/pkg/iter"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	query "github.com/grafana/phlare/pkg/phlaredb/query"
	schemav1 "github.com/grafana/phlare/pkg/phlaredb/schemas/v1"
	"github.com/grafana/phlare/pkg/phlaredb/tsdb"
	"github.com/grafana/phlare/pkg/phlaredb/tsdb/index"
)

type rowNum int64

func (r rowNum) RowNumber() int64 {
	return int64(r)
}

// delta encoding for ranges
type rowRange struct {
	rowNum int64
	length int
}

func (r *rowRange) iter() iter.Iterator[query.RowGetter] {
	return &rowRangeIter{
		rowRange: r,
		pos:      0,
	}
}

type rowRangeWithSeriesIndex struct {
	*rowRange
	seriesIndex uint32
}

// those need to be strictly ordered
type rowRangesWithSeriesIndex []rowRangeWithSeriesIndex

func (s rowRangesWithSeriesIndex) getSeriesIndex(rowNum int64) uint32 {
	// todo: binary search
	for _, rg := range s {
		if rg.rowNum <= rowNum && rg.rowNum+int64(rg.length) > rowNum {
			return rg.seriesIndex
		}
	}
	panic("series index not found")
}

type rowRangeIter struct {
	*rowRange
	pos rowNum
}

func (i *rowRangeIter) At() query.RowGetter {
	return i.pos - 1
}

func (i *rowRangeIter) Next() bool {
	if i.pos < rowNum(i.rowRange.rowNum) {
		i.pos = rowNum(i.rowRange.rowNum)
	}

	if i.pos >= rowNum(i.rowRange.rowNum)+rowNum(i.rowRange.length) {
		return false
	}
	i.pos++
	return true
}

func (i *rowRangeIter) Close() error { return nil }

func (i *rowRangeIter) Err() error { return nil }

type profileRowGroup struct {
	rowGroup *rowGroupOnDisk
	rowRange *rowRange
}

func (prg *profileRowGroup) rowNums() query.Iterator {
	return query.NewRowNumberIterator[query.RowGetter](prg.rowRange.iter())
}

func (prg *profileRowGroup) iter(ctx context.Context, start, end model.Time) query.Iterator {
	pIt := query.NewJoinIterator(
		0,
		[]query.Iterator{
			prg.rowNums(),
			prg.rowGroup.columnIter(ctx, "TimeNanos", query.NewIntBetweenPredicate(start.UnixNano(), end.UnixNano()), "TimeNanos"),
		},
		nil,
	)

	return pIt
}

type profileOnDisk struct {
	BlockProfile
	rowGroup *profileRowGroup
}

type profileLabels struct {
	lbs phlaremodel.Labels
	fp  model.Fingerprint

	minTime, maxTime int64

	// profiles in memory
	profiles []*schemav1.Profile

	// profiles temporary stored on disk in row group segements
	// TODO: this information is crucial to recover segements to a full block later
	profilesOnDisk []*profileRowGroup
}

func (pi *profileLabels) selectProfilesWithin(ctx context.Context, start, end model.Time) iter.Iterator[Profile] {
	var (
		profiles []Profile
		buf      = make([][]parquet.Value, 1)
		iters    = make([]iter.Iterator[Profile], 0, 2)
	)

	// get profiles from memory first
	iters = append(iters,
		NewSeriesIterator(
			pi.lbs,
			pi.fp,
			iter.NewTimeRangedIterator(iter.NewSliceIterator(pi.profiles), start, end),
		),
	)

	// now add profiles from disk row groups
	for _, rg := range pi.profilesOnDisk {
		pIt := rg.iter(ctx, start, end)
		for pIt.Next() {
			res := pIt.At()
			buf = res.Columns(buf, "TimeNanos")
			profiles = append(profiles, profileOnDisk{
				BlockProfile: BlockProfile{
					labels: pi.lbs,
					fp:     pi.fp,
					ts:     model.TimeFromUnixNano(buf[0][0].Int64()),
					RowNum: res.RowNumber[0],
				},
				rowGroup: rg,
			})
		}
		if err := pIt.Err(); err != nil {
			return iter.NewErrIterator[Profile](err)
		}
	}
	iters = append(iters, iter.NewSliceIterator(profiles))

	return iter.NewSortProfileIterator(iters)

}

func (pi *profileLabels) allProfilesFunc(rows []*schemav1.Profile, fn func(lbs phlaremodel.Labels, fp model.Fingerprint, profile *schemav1.Profile) error) error {
	// read profiles in memory first
	for _, p := range pi.profiles {
		if p.SeriesFingerprint == pi.fp {
			if err := fn(pi.lbs, pi.fp, p); err != nil {
				return err
			}
		}
	}

	// read profiles on disk
	for _, rg := range pi.profilesOnDisk {
		reader := parquet.NewGenericRowGroupReader[*schemav1.Profile](rg.rowGroup)

		if cap(rows) < rg.rowRange.length {
			rows = make([]*schemav1.Profile, rg.rowRange.length)
		} else {
			rows = rows[:rg.rowRange.length]
		}

		if err := reader.SeekToRow(rg.rowRange.rowNum); err != nil {
			return err
		}

		if _, err := reader.Read(rows); err != nil {
			return err
		}
		for _, p := range rows {
			if err := fn(pi.lbs, pi.fp, p); err != nil {
				return err
			}
		}
		if err := reader.Close(); err != nil {
			return err
		}
	}
	return nil
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
func (pi *profilesIndex) Add(ps *schemav1.Profile, lbs phlaremodel.Labels, profileName string) {
	pi.mutex.Lock()
	defer pi.mutex.Unlock()
	profiles, ok := pi.profilesPerFP[ps.SeriesFingerprint]
	if !ok {
		lbs := pi.ix.Add(lbs, ps.SeriesFingerprint)
		profiles = &profileLabels{
			lbs:     lbs,
			fp:      ps.SeriesFingerprint,
			minTime: ps.TimeNanos,
			maxTime: ps.TimeNanos,
		}
		pi.profilesPerFP[ps.SeriesFingerprint] = profiles
		pi.totalSeries.Inc()
		pi.metrics.seriesCreated.WithLabelValues(profileName).Inc()
	}
	profiles.profiles = append(profiles.profiles, ps)
	if ps.TimeNanos < profiles.minTime {
		profiles.minTime = ps.TimeNanos
	}
	if ps.TimeNanos > profiles.maxTime {
		profiles.maxTime = ps.TimeNanos
	}

	pi.totalProfiles.Inc()
	pi.metrics.profilesCreated.WithLabelValues(profileName).Inc()
}

// forMatchingProfiles iterates through all matching profiles and calls f for each profiles.
// The profile contains multiple samples not all of them are matching the matchers.
// You can use sampleIdx to filter the samples by his position in the returned profile.
// The returned profile is not sorted.
func (pi *profilesIndex) forMatchingProfiles(matchers []*labels.Matcher,
	fn func(lbs phlaremodel.Labels, fp model.Fingerprint, profile *schemav1.Profile) error,
) error {
	filters, matchers := SplitFiltersAndMatchers(matchers)
	ids, err := pi.ix.Lookup(matchers, nil)
	if err != nil {
		return err
	}

	pi.mutex.RLock()
	defer pi.mutex.RUnlock()

	rows := make([]*schemav1.Profile, 0, 100)
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

		if err := profile.allProfilesFunc(rows, fn); err != nil {
			return err
		}
	}
	return nil
}

func (pi *profilesIndex) SelectMatchingProfiles(ctx context.Context, params *ingestv1.SelectProfilesRequest) (iter.Iterator[Profile], error) {
	sp, _ := opentracing.StartSpanFromContext(ctx, "SelectMatchingProfiles - Index")
	defer sp.Finish()
	selectors, err := parser.ParseMetricSelector(params.LabelSelector)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "failed to parse label selectors: "+err.Error())
	}
	selectors = append(selectors, phlaremodel.SelectorFromProfileType(params.Type))

	filters, matchers := SplitFiltersAndMatchers(selectors)
	ids, err := pi.ix.Lookup(matchers, nil)
	if err != nil {
		return nil, err
	}

	start := model.Time(params.Start)
	end := model.Time(params.End)

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

		iters = append(iters, profile.selectProfilesWithin(ctx, start, end))
	}

	return iter.NewSortProfileIterator(iters), nil
}

type ProfileWithLabels struct {
	*schemav1.Profile
	lbs phlaremodel.Labels
	fp  model.Fingerprint
}

func (p ProfileWithLabels) Timestamp() model.Time {
	return model.TimeFromUnixNano(p.Profile.TimeNanos)
}

func (p ProfileWithLabels) Fingerprint() model.Fingerprint {
	return p.fp
}

func (p ProfileWithLabels) Labels() phlaremodel.Labels {
	return p.lbs
}

func (p ProfileWithLabels) Samples() []*schemav1.Sample {
	return p.Profile.Samples
}

type SeriesIterator struct {
	iter.Iterator[*schemav1.Profile]
	curr ProfileWithLabels
	fp   model.Fingerprint
	lbs  phlaremodel.Labels
}

func NewSeriesIterator(labels phlaremodel.Labels, fingerprint model.Fingerprint, it iter.Iterator[*schemav1.Profile]) *SeriesIterator {
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
	fn func(lbs phlaremodel.Labels, fp model.Fingerprint) error,
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

func (pi *profilesIndex) allProfilesFunc(rows []*schemav1.Profile, fn func(lbs phlaremodel.Labels, fp model.Fingerprint, profile *schemav1.Profile) error) error {
	pi.mutex.RLock()
	defer pi.mutex.RUnlock()

	for _, profile := range pi.profilesPerFP {
		if err := profile.allProfilesFunc(rows, fn); err != nil {
			return err
		}
	}

	return nil

}

func (pi *profilesIndex) allProfiles() ([]*schemav1.Profile, error) {
	total := pi.totalProfiles.Load()
	result := make([]*schemav1.Profile, 0, total)
	uniq := make(map[uuid.UUID]struct{}, total)

	rows := make([]*schemav1.Profile, 0, 100)
	if err := pi.allProfilesFunc(rows, func(_ phlaremodel.Labels, _ model.Fingerprint, profile *schemav1.Profile) error {
		if _, ok := uniq[profile.ID]; !ok {
			uniq[profile.ID] = struct{}{}
			result = append(result, profile)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return result, nil
}

// WriteTo writes the profiles tsdb index to the specified filepath.
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
		return phlaremodel.CompareLabelPairs(pfs[i].lbs, pfs[j].lbs) < 0
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

	// ranges per row group
	rangesPerRG := make(map[*rowGroupOnDisk][]rowRangeWithSeriesIndex)

	// Add series
	//pi.seriesIndexes = make(map[model.Fingerprint]uint32, len(pfs))
	for i, s := range pfs {
		if err := writer.AddSeries(storage.SeriesRef(i), s.lbs, s.fp, index.ChunkMeta{
			MinTime: s.minTime,
			MaxTime: s.maxTime,
			// We store the series Index from the head with the series to use when retrieving data from parquet.
			SeriesIndex: uint32(i),
		}); err != nil {
			return err
		}
		// store series index
		for _, rg := range s.profilesOnDisk {
			rangesPerRG[rg.rowGroup] = append(rangesPerRG[rg.rowGroup], rowRangeWithSeriesIndex{rowRange: rg.rowRange, seriesIndex: uint32(i)})
		}
	}

	// store information into the row groups
	for rg, rowRanges := range rangesPerRG {
		rg.seriesIndexes = rowRanges
	}

	return writer.Close()
}

func (pl *profilesIndex) cutRowGroup(rgProfiles []*schemav1.Profile, rowGroup *rowGroupOnDisk) error {
	// adding rowGroup and rowNum information per fingerprint
	var rowRangePerFP = make(map[model.Fingerprint]*rowRange, len(pl.profilesPerFP))
	for rowNum, p := range rgProfiles {
		if _, ok := rowRangePerFP[p.SeriesFingerprint]; !ok {
			rowRangePerFP[p.SeriesFingerprint] = &rowRange{
				rowNum: int64(rowNum),
			}
		}

		rowRange := rowRangePerFP[p.SeriesFingerprint]
		rowRange.length++

		// sanity check
		if (int(rowRange.rowNum) + rowRange.length - 1) != rowNum {
			return fmt.Errorf("rowRange is not matching up, ensure that the ordering of the profile row group is ordered correctly, current row_num=%d, expect range %d-%d", rowNum, rowRange.rowNum, int(rowRange.rowNum)+rowRange.length)
		}
	}

	pl.mutex.Lock()
	defer pl.mutex.Unlock()

	for _, ps := range pl.profilesPerFP {
		// empty all in memory profiles
		ps.profiles = ps.profiles[:0]

		// attach rowGroup and rowNum information
		rowRange, ok := rowRangePerFP[ps.fp]
		if !ok {
			continue
		}

		ps.profilesOnDisk = append(
			ps.profilesOnDisk,
			&profileRowGroup{
				rowGroup: rowGroup,
				rowRange: rowRange,
			},
		)
	}

	return nil

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
