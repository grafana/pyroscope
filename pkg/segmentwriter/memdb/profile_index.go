package memdb

import (
	"context"
	"sort"
	"sync"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage"
	"go.uber.org/atomic"

	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
	memindex "github.com/grafana/pyroscope/pkg/segmentwriter/memdb/index"
)

type profileSeries struct {
	lbs              phlaremodel.Labels
	fp               model.Fingerprint
	minTime, maxTime int64
	profiles         []*schemav1.InMemoryProfile
}

type profilesIndex struct {
	ix            *tsdb.BitPrefixInvertedIndex
	profilesPerFP map[model.Fingerprint]*profileSeries
	mutex         sync.RWMutex
	metrics       *HeadMetrics
	totalSeries   *atomic.Int64
}

func newProfileIndex(metrics *HeadMetrics) *profilesIndex {
	ix, err := tsdb.NewBitPrefixWithShards(32)
	if err != nil {
		panic(err)
	}
	return &profilesIndex{
		ix:            ix,
		profilesPerFP: make(map[model.Fingerprint]*profileSeries),
		metrics:       metrics,
		totalSeries:   atomic.NewInt64(0),
	}
}

// Add a new set of profile to the index.
// The seriesRef are expected to match the profile labels passed in.
func (pi *profilesIndex) Add(ps *schemav1.InMemoryProfile, lbs phlaremodel.Labels, profileName string) {
	pi.mutex.Lock()
	defer pi.mutex.Unlock()
	profiles, ok := pi.profilesPerFP[ps.SeriesFingerprint]
	if !ok {
		lbs := pi.ix.Add(lbs, ps.SeriesFingerprint)
		profiles = &profileSeries{
			lbs:     lbs,
			fp:      ps.SeriesFingerprint,
			minTime: ps.TimeNanos,
			maxTime: ps.TimeNanos,
		}
		pi.profilesPerFP[ps.SeriesFingerprint] = profiles
		//pi.metrics.series.Set(float64(pi.totalSeries.Inc())) // todo how did it work?
		pi.totalSeries.Inc()
		pi.metrics.seriesCreated.WithLabelValues(profileName).Inc()
	}

	// profile is latest in this series, use a shortcut
	if ps.TimeNanos > profiles.maxTime {
		// update max timeNanos
		profiles.maxTime = ps.TimeNanos

		// add profile to in memory slice
		profiles.profiles = append(profiles.profiles, ps)
	} else {
		// use binary search to find position
		i := sort.Search(len(profiles.profiles), func(i int) bool {
			return profiles.profiles[i].TimeNanos > ps.TimeNanos
		})

		// insert into slice at correct position
		profiles.profiles = append(profiles.profiles, &schemav1.InMemoryProfile{})
		copy(profiles.profiles[i+1:], profiles.profiles[i:])
		profiles.profiles[i] = ps
	}

	if ps.TimeNanos < profiles.minTime {
		profiles.minTime = ps.TimeNanos
	}

	//pi.metrics.profiles.Set(float64(pi.totalProfiles.Inc())) //todo how did it work?
	pi.metrics.profilesCreated.WithLabelValues(profileName).Inc()
}

func (pi *profilesIndex) Flush(ctx context.Context) ([]byte, []schemav1.InMemoryProfile, error) {
	writer, err := memindex.NewWriter(ctx, memindex.SegmentsIndexWriterBufSize)
	if err != nil {
		return nil, nil, err
	}
	pi.mutex.RLock()
	defer pi.mutex.RUnlock()

	// TODO(kolesnikovae): We should reuse these series
	//   when building dataset index.
	pfs := make([]*profileSeries, 0, len(pi.profilesPerFP))
	profilesSize := 0

	for _, p := range pi.profilesPerFP {
		pfs = append(pfs, p)
		profilesSize += len(p.profiles)
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
			return nil, nil, err
		}
	}

	profiles := make([]schemav1.InMemoryProfile, 0, profilesSize)

	// Add series
	for i, s := range pfs {
		if err := writer.AddSeries(storage.SeriesRef(i), s.lbs, s.fp, index.ChunkMeta{
			MinTime: s.minTime,
			MaxTime: s.maxTime,
			// We store the series Index from the head with the series to use when retrieving data from parquet.
			SeriesIndex: uint32(i),
		}); err != nil {
			return nil, nil, err
		}
		// store series index
		for j := range s.profiles {
			s.profiles[j].SeriesIndex = uint32(i)
		}
		//profiles = append(profiles, s.profiles...)
		for _, profile := range s.profiles {
			profiles = append(profiles, *profile) //todo avoid copy
		}
	}

	err = writer.Close()
	if err != nil {
		return nil, nil, err
	}

	//todo maybe return the bufferWriter to avoid copy, it is copied again anyway
	tsdbIndex := writer.ReleaseIndex()

	return tsdbIndex, profiles, err
}

func (pi *profilesIndex) profileTypeNames() ([]string, error) {
	pi.mutex.RLock()
	defer pi.mutex.RUnlock()
	ptypes, err := pi.ix.LabelValues(phlaremodel.LabelNameProfileType, nil)
	sort.Strings(ptypes)
	return ptypes, err
}
