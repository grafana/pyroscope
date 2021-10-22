package storage

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage/dimension"
	"github.com/pyroscope-io/pyroscope/pkg/storage/labels"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
	"github.com/pyroscope-io/pyroscope/pkg/util/slices"
)

var (
	errRetention = errors.New("could not write because of retention settings")
	errClosed    = errors.New("storage closed")
)

var (
	maxTime  = time.Unix(1<<62, 999999999)
	zeroTime time.Time
)

type Storage struct {
	*options

	config *config.Server
	logger *logrus.Logger

	main       *db
	segments   *db
	dimensions *db
	dicts      *db
	trees      *db
	labels     *labels.Labels

	*metrics
	*cacheMetrics

	size bytesize.ByteSize

	// Maintenance tasks are executed exclusively to avoid competition:
	// extensive writing during GC is harmful and deteriorates the
	// overall performance. Same for write back, eviction, and retention
	// tasks.
	maintenance sync.Mutex
	stop        chan struct{}
	wg          sync.WaitGroup

	putMutex sync.Mutex
}

type prefix string

const (
	segmentPrefix    prefix = "s:"
	treePrefix       prefix = "t:"
	dictionaryPrefix prefix = "d:"
	dimensionPrefix  prefix = "i:"
)

func (p prefix) String() string      { return string(p) }
func (p prefix) bytes() []byte       { return []byte(p) }
func (p prefix) key(k string) []byte { return []byte(string(p) + k) }

func (p prefix) trim(k []byte) ([]byte, bool) {
	if len(k) > len(p) {
		return k[len(p):], true
	}
	return nil, false
}

func New(c *config.Server, logger *logrus.Logger, reg prometheus.Registerer, options ...Option) (*Storage, error) {
	s := &Storage{
		config:  c,
		options: defaultOptions(),

		logger:       logger,
		stop:         make(chan struct{}),
		metrics:      newStorageMetrics(reg),
		cacheMetrics: newCacheMetrics(reg),
	}

	for _, option := range options {
		option(s)
	}

	var err error
	if s.main, err = s.newBadger("main", "", nil); err != nil {
		return nil, err
	}
	if s.dicts, err = s.newBadger("dicts", dictionaryPrefix, dictionaryCodec{}); err != nil {
		return nil, err
	}
	if s.dimensions, err = s.newBadger("dimensions", dimensionPrefix, dimensionCodec{}); err != nil {
		return nil, err
	}
	if s.segments, err = s.newBadger("segments", segmentPrefix, segmentCodec{}); err != nil {
		return nil, err
	}
	if s.trees, err = s.newBadger("trees", treePrefix, treeCodec{s}); err != nil {
		return nil, err
	}

	s.labels = labels.New(s.main.DB)

	if err = s.migrate(); err != nil {
		return nil, err
	}

	// TODO(kolesnikovae): Allow failure and skip evictionTask?
	memTotal, err := getMemTotal()
	if err != nil {
		return nil, err
	}

	// TODO(kolesnikovae): Make it possible to run CollectGarbage
	//  without starting any other maintenance tasks at server start.
	s.wg.Add(4)
	go s.maintenanceTask(s.gcInterval, s.watchDBSize(s.gcSizeDiff, s.CollectGarbage))
	go s.maintenanceTask(s.evictInterval, s.evictionTask(memTotal))
	go s.maintenanceTask(s.writeBackInterval, s.writeBackTask)
	go s.periodicTask(s.writeBackInterval, s.updateMetricsTask)

	return s, nil
}

func (s *Storage) Close() error {
	// Stop all periodic and maintenance tasks.
	close(s.stop)
	s.wg.Wait()
	// Dictionaries DB has to close last because trees depend on it.
	s.goDB(func(d *db) {
		if d != s.dicts {
			d.close()
		}
	})
	s.dicts.close()
	return nil
}

type PutInput struct {
	StartTime       time.Time
	EndTime         time.Time
	Key             *segment.Key
	Val             *tree.Tree
	SpyName         string
	SampleRate      uint32
	Units           string
	AggregationType string
}

func (s *Storage) Put(pi *PutInput) error {
	// TODO: This is a pretty broad lock. We should find a way to make these locks more selective.
	s.putMutex.Lock()
	defer s.putMutex.Unlock()

	if pi.StartTime.Before(s.retentionPolicy().LowerTimeBoundary()) {
		return errRetention
	}

	s.putTotal.Inc()
	s.logger.WithFields(logrus.Fields{
		"startTime":       pi.StartTime.String(),
		"endTime":         pi.EndTime.String(),
		"key":             pi.Key.Normalized(),
		"samples":         pi.Val.Samples(),
		"units":           pi.Units,
		"aggregationType": pi.AggregationType,
	}).Debug("storage.Put")

	for k, v := range pi.Key.Labels() {
		s.labels.Put(k, v)
	}

	sk := pi.Key.SegmentKey()
	for k, v := range pi.Key.Labels() {
		key := k + ":" + v
		r, err := s.dimensions.GetOrCreate(key)
		if err != nil {
			s.logger.Errorf("dimensions cache for %v: %v", key, err)
			continue
		}
		r.(*dimension.Dimension).Insert([]byte(sk))
		s.dimensions.Put(key, r)
	}

	r, err := s.segments.GetOrCreate(sk)
	if err != nil {
		return fmt.Errorf("segments cache for %v: %v", sk, err)
	}

	st := r.(*segment.Segment)
	st.SetMetadata(pi.SpyName, pi.SampleRate, pi.Units, pi.AggregationType)
	samples := pi.Val.Samples()

	err = st.Put(pi.StartTime, pi.EndTime, samples, func(depth int, t time.Time, r *big.Rat, addons []segment.Addon) {
		tk := pi.Key.TreeKey(depth, t)
		res, err := s.trees.GetOrCreate(tk)
		if err != nil {
			s.logger.Errorf("trees cache for %v: %v", tk, err)
			return
		}
		cachedTree := res.(*tree.Tree)
		treeClone := pi.Val.Clone(r)
		for _, addon := range addons {
			if res, ok := s.trees.Lookup(pi.Key.TreeKey(addon.Depth, addon.T)); ok {
				ta := res.(*tree.Tree)
				ta.RLock()
				treeClone.Merge(ta)
				ta.RUnlock()
			}
		}
		cachedTree.Lock()
		cachedTree.Merge(treeClone)
		cachedTree.Unlock()
		s.trees.Put(tk, cachedTree)
	})
	if err != nil {
		return err
	}

	s.segments.Put(sk, st)
	return nil
}

type GetInput struct {
	StartTime time.Time
	EndTime   time.Time
	Key       *segment.Key
	Query     *flameql.Query
}

type GetOutput struct {
	Tree       *tree.Tree
	Timeline   *segment.Timeline
	SpyName    string
	SampleRate uint32
	Units      string
}

const averageAggregationType = "average"

func (s *Storage) Get(gi *GetInput) (*GetOutput, error) {
	logger := logrus.WithFields(logrus.Fields{
		"startTime": gi.StartTime.String(),
		"endTime":   gi.EndTime.String(),
	})

	var dimensionKeys func() []dimension.Key
	switch {
	case gi.Key != nil:
		logger = logger.WithField("key", gi.Key.Normalized())
		dimensionKeys = s.dimensionKeysByKey(gi.Key)
	case gi.Query != nil:
		logger = logger.WithField("query", gi.Query)
		dimensionKeys = s.dimensionKeysByQuery(gi.Query)
	default:
		// Should never happen.
		return nil, fmt.Errorf("key or query must be specified")
	}

	s.getTotal.Inc()
	logger.Debug("storage.Get")

	var (
		resultTrie  *tree.Tree
		lastSegment *segment.Segment
		writesTotal uint64

		aggregationType = "sum"
		timeline        = segment.GenerateTimeline(gi.StartTime, gi.EndTime)
		threshold       = s.retentionPolicy()
	)

	for _, k := range dimensionKeys() {
		// TODO: refactor, store `Key`s in dimensions
		parsedKey, err := segment.ParseKey(string(k))
		if err != nil {
			s.logger.Errorf("parse key: %v: %v", string(k), err)
			continue
		}
		key := parsedKey.SegmentKey()
		res, ok := s.segments.Lookup(key)
		if !ok {
			continue
		}

		st := res.(*segment.Segment)
		if st.AggregationType() == averageAggregationType {
			aggregationType = averageAggregationType
		}

		timeline.PopulateTimeline(st, threshold)
		lastSegment = st

		st.Get(gi.StartTime, gi.EndTime, func(depth int, samples, writes uint64, t time.Time, r *big.Rat) {
			if res, ok = s.trees.Lookup(parsedKey.TreeKey(depth, t)); ok {
				x := res.(*tree.Tree).Clone(r)
				writesTotal += writes
				if resultTrie == nil {
					resultTrie = x
					return
				}
				resultTrie.Merge(x)
			}
		})
	}

	if resultTrie == nil || lastSegment == nil {
		return nil, nil
	}

	if writesTotal > 0 && aggregationType == averageAggregationType {
		resultTrie = resultTrie.Clone(big.NewRat(1, int64(writesTotal)))
	}

	return &GetOutput{
		Tree:       resultTrie,
		Timeline:   timeline,
		SpyName:    lastSegment.SpyName(),
		SampleRate: lastSegment.SampleRate(),
		Units:      lastSegment.Units(),
	}, nil
}

func (s *Storage) dimensionKeysByKey(key *segment.Key) func() []dimension.Key {
	return func() []dimension.Key {
		d, ok := s.lookupAppDimension(key.AppName())
		if !ok {
			return nil
		}
		l := key.Labels()
		if len(l) == 1 {
			// No tags specified: return application dimension keys.
			return d.Keys
		}
		dimensions := []*dimension.Dimension{d}
		for k, v := range l {
			if flameql.IsTagKeyReserved(k) {
				continue
			}
			if d, ok = s.lookupDimensionKV(k, v); ok {
				dimensions = append(dimensions, d)
			}
		}
		if len(dimensions) == 1 {
			// Tags specified but not found.
			return nil
		}
		return dimension.Intersection(dimensions...)
	}
}

func (s *Storage) dimensionKeysByQuery(qry *flameql.Query) func() []dimension.Key {
	return func() []dimension.Key { return s.exec(context.TODO(), qry) }
}

type DeleteInput struct {
	// Key must match exactly one segment.
	Key *segment.Key
}

func (s *Storage) Delete(di *DeleteInput) error {
	return s.deleteSegmentAndRelatedData(di.Key)
}

func (s *Storage) deleteSegmentAndRelatedData(k *segment.Key) error {
	sk := k.SegmentKey()
	if _, ok := s.segments.Lookup(sk); !ok {
		return nil
	}
	// Drop trees from disk.
	if err := s.trees.DropPrefix(treePrefix.key(sk)); err != nil {
		return err
	}
	// Discarding cached items is necessary because otherwise those would
	// be written back to disk on eviction.
	s.trees.DiscardPrefix(sk)
	// Only remove dictionary if there are no more segments referencing it.
	if apps, ok := s.lookupAppDimension(k.AppName()); ok && len(apps.Keys) == 1 {
		if err := s.dicts.Delete(k.DictKey()); err != nil {
			return err
		}
	}
	for key, value := range k.Labels() {
		if d, ok := s.lookupDimensionKV(key, value); ok {
			d.Delete(dimension.Key(sk))
		}
	}
	return s.segments.Delete(sk)
}

func (s *Storage) GetKeys(cb func(string) bool) { s.labels.GetKeys(cb) }

func (s *Storage) GetValues(key string, cb func(v string) bool) {
	s.labels.GetValues(key, func(v string) bool {
		if key != "__name__" || !slices.StringContains(s.config.HideApplications, v) {
			return cb(v)
		}
		return true
	})
}

func (s *Storage) GetKeysByQuery(query string, cb func(_k string) bool) error {
	parsedQuery, err := flameql.ParseQuery(query)
	if err != nil {
		return err
	}

	segmentKey, err := segment.ParseKey(parsedQuery.AppName + "{}")
	if err != nil {
		return err
	}
	dimensionKeys := s.dimensionKeysByKey(segmentKey)

	resultSet := map[string]bool{}
	for _, dk := range dimensionKeys() {
		dkParsed, _ := segment.ParseKey(string(dk))
		if dkParsed.AppName() == parsedQuery.AppName {
			for k := range dkParsed.Labels() {
				resultSet[k] = true
			}
		}
	}

	resultList := []string{}
	for v := range resultSet {
		resultList = append(resultList, v)
	}

	sort.Strings(resultList)
	for _, v := range resultList {
		if !cb(v) {
			break
		}
	}
	return nil
}

func (s *Storage) GetValuesByQuery(label string, query string, cb func(v string) bool) error {
	parsedQuery, err := flameql.ParseQuery(query)
	if err != nil {
		return err
	}

	segmentKey, err := segment.ParseKey(parsedQuery.AppName + "{}")
	if err != nil {
		return err
	}
	dimensionKeys := s.dimensionKeysByKey(segmentKey)

	resultSet := map[string]bool{}
	for _, dk := range dimensionKeys() {
		dkParsed, _ := segment.ParseKey(string(dk))
		if v, ok := dkParsed.Labels()[label]; ok {
			resultSet[v] = true
		}
	}

	resultList := []string{}
	for v := range resultSet {
		resultList = append(resultList, v)
	}

	sort.Strings(resultList)
	for _, v := range resultList {
		if !cb(v) {
			break
		}
	}
	return nil
}

func (s *Storage) DiskUsage() map[string]bytesize.ByteSize {
	m := make(map[string]bytesize.ByteSize)
	for _, d := range s.databases() {
		m[d.name] = dbSize(d)
	}
	return m
}

func (s *Storage) CacheStats() map[string]interface{} {
	m := make(map[string]interface{})
	for _, d := range s.databases() {
		if d.Cache != nil {
			m[d.name+"_size"] = s.dimensions.Cache.Size()
		}
	}
	return m
}
