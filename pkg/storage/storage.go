package storage

import (
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage/cache"
	"github.com/pyroscope-io/pyroscope/pkg/storage/dict"
	"github.com/pyroscope-io/pyroscope/pkg/storage/dimension"
	"github.com/pyroscope-io/pyroscope/pkg/storage/labels"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/merge"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
	"github.com/sirupsen/logrus"
)

var closingErr = errors.New("the db is in closing state")

type Storage struct {
	closingMutex sync.Mutex
	closing      bool

	cfg      *config.Config
	segments *cache.Cache

	dimensions *cache.Cache
	dicts      *cache.Cache
	trees      *cache.Cache
	labels     *labels.Labels

	db           *badger.DB
	dbTrees      *badger.DB
	dbDicts      *badger.DB
	dbDimensions *badger.DB
	dbSegments   *badger.DB
}

func badgerGC(db *badger.DB) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
	again:
		err := db.RunValueLogGC(0.7)
		if err == nil {
			goto again
		}
	}
}

func newBadger(cfg *config.Config, name string) (*badger.DB, error) {
	badgerPath := filepath.Join(cfg.Server.StoragePath, name)
	err := os.MkdirAll(badgerPath, 0755)
	if err != nil {
		return nil, err
	}
	badgerOptions := badger.DefaultOptions(badgerPath)
	badgerOptions = badgerOptions.WithTruncate(false)
	badgerOptions = badgerOptions.WithSyncWrites(false)
	badgerOptions = badgerOptions.WithCompression(options.ZSTD)
	badgerLevel := logrus.ErrorLevel
	if l, err := logrus.ParseLevel(cfg.Server.BadgerLogLevel); err == nil {
		badgerLevel = l
	}
	badgerOptions = badgerOptions.WithLogger(badgerLogger{name: name, logLevel: badgerLevel})

	db, err := badger.Open(badgerOptions)
	if err == nil {
		go badgerGC(db)
	}
	return db, err
}

func New(cfg *config.Config) (*Storage, error) {
	db, err := newBadger(cfg, "main")
	if err != nil {
		return nil, err
	}
	dbTrees, err := newBadger(cfg, "trees")
	if err != nil {
		return nil, err
	}
	dbDicts, err := newBadger(cfg, "dicts")
	if err != nil {
		return nil, err
	}
	dbDimensions, err := newBadger(cfg, "dimensions")
	if err != nil {
		return nil, err
	}
	dbSegments, err := newBadger(cfg, "segments")
	if err != nil {
		return nil, err
	}

	s := &Storage{
		cfg:          cfg,
		labels:       labels.New(cfg, db),
		db:           db,
		dbTrees:      dbTrees,
		dbDicts:      dbDicts,
		dbDimensions: dbDimensions,
		dbSegments:   dbSegments,
	}

	s.dimensions = cache.New(dbDimensions, cfg.Server.CacheDimensionSize, "i:")
	s.dimensions.Bytes = func(_k string, v interface{}) []byte {
		return v.(*dimension.Dimension).Bytes()
	}
	s.dimensions.FromBytes = func(_k string, v []byte) interface{} {
		return dimension.FromBytes(v)
	}
	s.dimensions.New = func(_k string) interface{} {
		return dimension.New()
	}

	s.segments = cache.New(dbSegments, cfg.Server.CacheSegmentSize, "s:")
	s.segments.Bytes = func(_k string, v interface{}) []byte {
		return v.(*segment.Segment).Bytes()
	}
	s.segments.FromBytes = func(_k string, v []byte) interface{} {
		// TODO:
		//   these configuration params should be saved in db when it initializes
		return segment.FromBytes(cfg.Server.MinResolution, cfg.Server.Multiplier, v)
	}
	s.segments.New = func(_k string) interface{} {
		return segment.New(s.cfg.Server.MinResolution, s.cfg.Server.Multiplier)
	}

	s.dicts = cache.New(dbDicts, cfg.Server.CacheDictionarySize, "d:")
	s.dicts.Bytes = func(_k string, v interface{}) []byte {
		return v.(*dict.Dict).Bytes()
	}
	s.dicts.FromBytes = func(_k string, v []byte) interface{} {
		return dict.FromBytes(v)
	}
	s.dicts.New = func(_k string) interface{} {
		return dict.New()
	}

	s.trees = cache.New(dbTrees, cfg.Server.CacheSegmentSize, "t:")
	s.trees.Bytes = func(k string, v interface{}) []byte {
		d := s.dicts.Get(FromTreeToMainKey(k)).(*dict.Dict)
		return v.(*tree.Tree).Bytes(d, cfg.Server.MaxNodesSerialization)
	}
	s.trees.FromBytes = func(k string, v []byte) interface{} {
		d := s.dicts.Get(FromTreeToMainKey(k)).(*dict.Dict)
		return tree.FromBytes(d, v)
	}
	s.trees.New = func(_k string) interface{} {
		return tree.New()
	}

	// TODO: horrible, remove soon
	segment.InitializeGlobalState(s.cfg.Server.MinResolution, s.cfg.Server.Multiplier)

	return s, nil
}

func (s *Storage) Put(startTime, endTime time.Time, key *Key, val *tree.Tree, spyName string, sampleRate int) error {
	s.closingMutex.Lock()
	defer s.closingMutex.Unlock()

	if s.closing {
		return closingErr
	}
	logrus.WithFields(logrus.Fields{
		"startTime": startTime.String(),
		"endTime":   endTime.String(),
		"key":       key.Normalized(),
		"samples":   val.Samples(),
	}).Info("storage.Put")
	for k, v := range key.labels {
		s.labels.Put(k, v)
	}

	sk := key.SegmentKey()
	for k, v := range key.labels {
		d := s.dimensions.Get(k + ":" + v).(*dimension.Dimension)
		d.Insert([]byte(sk))
	}

	st := s.segments.Get(sk).(*segment.Segment)
	st.SetMetadata(spyName, sampleRate)
	samples := val.Samples()
	st.Put(startTime, endTime, samples, func(depth int, t time.Time, r *big.Rat, addons []segment.Addon) {
		tk := key.TreeKey(depth, t)
		existingTree := s.trees.Get(tk).(*tree.Tree)
		treeClone := val.Clone(big.NewRat(1, 1))
		for _, addon := range addons {
			tk2 := key.TreeKey(addon.Depth, addon.T)
			addonTree := s.trees.Get(tk2).(*tree.Tree)
			treeClone.Merge(addonTree)
		}
		if existingTree != nil {
			existingTree.Merge(treeClone)
			s.trees.Put(tk, existingTree)
		} else {
			s.trees.Put(tk, treeClone)
		}
	})
	s.segments.Put(string(sk), st)

	return nil
}

func (s *Storage) Get(startTime, endTime time.Time, key *Key) (*tree.Tree, *segment.Timeline, string, int, error) {
	s.closingMutex.Lock()
	defer s.closingMutex.Unlock()

	if s.closing {
		return nil, nil, "", 100, closingErr
	}

	logrus.WithFields(logrus.Fields{
		"startTime": startTime.String(),
		"endTime":   endTime.String(),
		"key":       key.Normalized(),
	}).Info("storage.Get")
	triesToMerge := []merge.Merger{}

	dimensions := []*dimension.Dimension{}
	for k, v := range key.labels {
		d := s.dimensions.Get(k + ":" + v).(*dimension.Dimension)
		dimensions = append(dimensions, d)
	}

	segmentKeys := dimension.Intersection(dimensions...)

	tl := segment.GenerateTimeline(startTime, endTime)
	var lastSegment *segment.Segment
	for _, sk := range segmentKeys {
		// TODO: refactor, store `Key`s in dimensions
		skk, _ := ParseKey(string(sk))
		st := s.segments.Get(skk.SegmentKey()).(*segment.Segment)
		if st == nil {
			continue
		}

		lastSegment = st

		tl.PopulateTimeline(startTime, endTime, st)

		st.Get(startTime, endTime, func(depth int, t time.Time, r *big.Rat) {
			k := skk.TreeKey(depth, t)
			tr := s.trees.Get(k).(*tree.Tree)
			// TODO: these clones are probably are not the most efficient way of doing this
			//   instead this info should be passed to the merger function imo
			tr2 := tr.Clone(r)
			triesToMerge = append(triesToMerge, merge.Merger(tr2))
		})
	}

	resultTrie := merge.MergeTriesConcurrently(runtime.NumCPU(), triesToMerge...)
	if resultTrie == nil {
		return nil, tl, "", 100, nil
	}
	return resultTrie.(*tree.Tree), tl, lastSegment.SpyName(), lastSegment.SampleRate(), nil
}

func (s *Storage) Close() error {
	s.closingMutex.Lock()
	s.closing = true
	s.closingMutex.Unlock()

	wg := sync.WaitGroup{}
	wg.Add(3)
	go func() { s.dimensions.Flush(); wg.Done() }()
	go func() { s.segments.Flush(); wg.Done() }()
	go func() { s.trees.Flush(); wg.Done() }()
	wg.Wait()
	// dictionary has to flush last because trees write to dictionaries
	s.dicts.Flush()
	s.dbTrees.Close()
	s.dbDicts.Close()
	s.dbDimensions.Close()
	s.dbSegments.Close()
	return s.db.Close()
}

func (s *Storage) GetKeys(cb func(_k string) bool) {
	s.labels.GetKeys(cb)
}

func (s *Storage) GetValues(key string, cb func(v string) bool) {
	s.labels.GetValues(key, cb)
}

func (s *Storage) DiskUsage() map[string]bytesize.ByteSize {
	res := map[string]bytesize.ByteSize{
		"main":       0,
		"trees":      0,
		"dicts":      0,
		"dimensions": 0,
		"segments":   0,
	}
	for k, _ := range res {
		res[k] = dirSize(filepath.Join(s.cfg.Server.StoragePath, k))
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
	return
}
