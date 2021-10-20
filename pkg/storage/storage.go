package storage

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage/cache"
	"github.com/pyroscope-io/pyroscope/pkg/storage/dimension"
	"github.com/pyroscope-io/pyroscope/pkg/storage/labels"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
	"github.com/pyroscope-io/pyroscope/pkg/util/disk"
	"github.com/pyroscope-io/pyroscope/pkg/util/slices"
)

var (
	errOutOfSpace = errors.New("running out of space")
	errRetention  = errors.New("could not write because of retention settings")

	badgerGCInterval  = 5 * time.Minute
	cacheTTL          = 2 * time.Minute
	writeBackInterval = time.Minute
	retentionInterval = 5 * time.Second // time.Minute
	evictInterval     = 20 * time.Second
)

type Storage struct {
	putMutex    sync.Mutex
	maintenance sync.Mutex

	config *config.Server

	// TODO(kolesnikovae): don't use global logger.
	// log    *logrus.Logger

	// TODO(kolesnikovae): unused, to be removed.
	localProfilesDir string

	db           *badger.DB
	dbTrees      *badger.DB
	dbDicts      *badger.DB
	dbDimensions *badger.DB
	dbSegments   *badger.DB

	labels     *labels.Labels
	segments   *cache.Cache
	dimensions *cache.Cache
	dicts      *cache.Cache
	trees      *cache.Cache

	*metrics

	stop chan struct{}
	wg   sync.WaitGroup
}

func (s *Storage) newBadger(name string) (*badger.DB, error) {
	badgerPath := filepath.Join(s.config.StoragePath, name)
	err := os.MkdirAll(badgerPath, 0o755)
	if err != nil {
		return nil, err
	}
	badgerOptions := badger.DefaultOptions(badgerPath)
	badgerOptions = badgerOptions.WithTruncate(!s.config.BadgerNoTruncate)
	badgerOptions = badgerOptions.WithSyncWrites(false)
	badgerOptions = badgerOptions.WithCompactL0OnClose(false)
	badgerOptions = badgerOptions.WithCompression(options.ZSTD)
	badgerLevel := logrus.ErrorLevel
	if l, err := logrus.ParseLevel(s.config.BadgerLogLevel); err == nil {
		badgerLevel = l
	}
	badgerOptions = badgerOptions.WithLogger(badgerLogger{name: name, logLevel: badgerLevel})
	db, err := badger.Open(badgerOptions)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func New(c *config.Server, reg prometheus.Registerer) (*Storage, error) {
	s := &Storage{
		config:           c,
		stop:             make(chan struct{}),
		localProfilesDir: filepath.Join(c.StoragePath, "local-profiles"),
		metrics:          newStorageMetrics(reg),
	}

	err := os.MkdirAll(s.localProfilesDir, 0o755)
	if err != nil {
		return nil, err
	}
	if s.db, err = s.newBadger("main"); err != nil {
		return nil, err
	}
	s.labels = labels.New(s.db)

	if s.dbTrees, err = s.newBadger("trees"); err != nil {
		return nil, err
	}
	if s.dbDicts, err = s.newBadger("dicts"); err != nil {
		return nil, err
	}
	if s.dbDimensions, err = s.newBadger("dimensions"); err != nil {
		return nil, err
	}
	if s.dbSegments, err = s.newBadger("segments"); err != nil {
		return nil, err
	}

	cm := newCacheMetrics(reg)

	s.dimensions = cache.New(cache.Config{
		DB:      s.dbDimensions,
		Metrics: cm.createInstance("dimensions"),
		Codec:   dimensionCodec{},
		Prefix:  "i:",
		TTL:     cacheTTL,
	})

	s.segments = cache.New(cache.Config{
		DB:      s.dbSegments,
		Metrics: cm.createInstance("segments"),
		Codec:   segmentCodec{},
		Prefix:  "s:",
		TTL:     cacheTTL,
	})

	s.dicts = cache.New(cache.Config{
		DB:      s.dbDicts,
		Metrics: cm.createInstance("dicts"),
		Codec:   dictionaryCodec{},
		Prefix:  "d:",
		TTL:     cacheTTL,
	})

	s.trees = cache.New(cache.Config{
		DB:      s.dbTrees,
		Metrics: cm.createInstance("trees"),
		Codec:   treeCodec{s},
		Prefix:  "t:",
		TTL:     cacheTTL,
	})

	memTotal, err := getMemTotal()
	if err != nil {
		return nil, err
	}

	if err = s.migrate(); err != nil {
		return nil, err
	}

	s.wg.Add(4)
	go s.periodicTask(evictInterval, s.evictionTask(memTotal))
	go s.periodicTask(writeBackInterval, s.writeBackTask)
	go s.periodicTask(retentionInterval, s.retentionTask)
	go s.periodicTask(badgerGCInterval, s.badgerGCTask)
	return s, nil
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

var (
	OutOfSpaceThreshold = 512 * bytesize.MB
	maxTime             = time.Unix(1<<62, 999999999)
	zeroTime            time.Time
)

func (s *Storage) Put(pi *PutInput) error {
	// TODO: This is a pretty broad lock. We should find a way to make these locks more selective.
	s.putMutex.Lock()
	defer s.putMutex.Unlock()

	if err := s.performFreeSpaceCheck(); err != nil {
		return err
	}

	if pi.StartTime.Before(s.retentionPolicy().LowerTimeBoundary()) {
		return errRetention
	}

	logrus.WithFields(logrus.Fields{
		"startTime":       pi.StartTime.String(),
		"endTime":         pi.EndTime.String(),
		"key":             pi.Key.Normalized(),
		"samples":         pi.Val.Samples(),
		"units":           pi.Units,
		"aggregationType": pi.AggregationType,
	}).Debug("storage.Put")

	s.writesTotal.Add(1.0)

	for k, v := range pi.Key.Labels() {
		s.labels.Put(k, v)
	}

	sk := pi.Key.SegmentKey()
	for k, v := range pi.Key.Labels() {
		key := k + ":" + v
		r, err := s.dimensions.GetOrCreate(key)
		if err != nil {
			logrus.Errorf("dimensions cache for %v: %v", key, err)
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
			logrus.Errorf("trees cache for %v: %v", tk, err)
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

	logger.Debug("storage.Get")

	s.readsTotal.Add(1)

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
			logrus.Errorf("parse key: %v: %v", string(k), err)
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

	if resultTrie == nil {
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
	// Keys must match exactly one segment.
	Keys []*segment.Key
}

func (s *Storage) Delete(di *DeleteInput) error {
	for _, sk := range di.Keys {
		if err := s.deleteSegmentAndRelatedData(sk); err != nil {
			return err
		}
	}
	return nil
}

func (s *Storage) deleteSegmentAndRelatedData(k *segment.Key) error {
	sk := k.SegmentKey()
	if _, ok := s.segments.Lookup(sk); !ok {
		return nil
	}
	// Drop trees from disk and cache.
	if err := s.dbTrees.DropPrefix([]byte("t:" + sk)); err != nil {
		return err
	}
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
	return s.segments.Delete(k.SegmentKey())
}

func (s *Storage) Close() error {
	close(s.stop)
	s.wg.Wait()

	func() {
		timer := prometheus.NewTimer(prometheus.ObserverFunc(s.cacheFlushTimer.Observe))
		defer timer.ObserveDuration()

		wg := sync.WaitGroup{}
		wg.Add(3)
		go func() { defer wg.Done(); s.dimensions.Flush() }()
		go func() { defer wg.Done(); s.segments.Flush() }()
		go func() { defer wg.Done(); s.trees.Flush() }()
		wg.Wait()

		// dictionary has to flush last because trees write to dictionaries
		s.dicts.Flush()
	}()

	func() {
		timer := prometheus.NewTimer(prometheus.ObserverFunc(s.badgerCloseTimer.Observe))
		defer timer.ObserveDuration()

		wg := sync.WaitGroup{}
		wg.Add(5)
		go func() { defer wg.Done(); s.dbTrees.Close() }()
		go func() { defer wg.Done(); s.dbDicts.Close() }()
		go func() { defer wg.Done(); s.dbDimensions.Close() }()
		go func() { defer wg.Done(); s.dbSegments.Close() }()
		go func() { defer wg.Done(); s.db.Close() }()
		wg.Wait()
	}()

	// this allows prometheus to collect metrics before pyroscope exits
	if os.Getenv("PYROSCOPE_WAIT_AFTER_STOP") != "" {
		time.Sleep(5 * time.Second)
	}
	return nil
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
	res := map[string]bytesize.ByteSize{
		"main":       0,
		"trees":      0,
		"dicts":      0,
		"dimensions": 0,
		"segments":   0,
	}
	for k := range res {
		res[k] = dirSize(filepath.Join(s.config.StoragePath, k))
	}
	return res
}

func dirSize(path string) (result bytesize.ByteSize) {
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			result += bytesize.ByteSize(info.Size())
		}
		return nil
	})
	return result
}

func (s *Storage) CacheStats() map[string]interface{} {
	return map[string]interface{}{
		"dimensions_size": s.dimensions.Size(),
		"segments_size":   s.segments.Size(),
		"dicts_size":      s.dicts.Size(),
		"trees_size":      s.trees.Size(),
	}
}

func (s *Storage) retentionPolicy() *segment.RetentionPolicy {
	t := segment.NewRetentionPolicy().
		SetAbsoluteMaxAge(s.config.Retention).
		SetAbsoluteSize(s.config.RetentionSize.Bytes())
	for level, threshold := range s.config.RetentionLevels {
		t.SetLevelMaxAge(level, threshold)
	}
	return t
}

func (s *Storage) performFreeSpaceCheck() error {
	freeSpace, err := disk.FreeSpace(s.config.StoragePath)
	if err == nil {
		if freeSpace < OutOfSpaceThreshold {
			return errOutOfSpace
		}
	}
	return nil
}
