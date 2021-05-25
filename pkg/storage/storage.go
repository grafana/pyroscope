package storage

import (
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/util/disk"
	"github.com/shirou/gopsutil/mem"

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
	"github.com/pyroscope-io/pyroscope/pkg/util/slices"
	"github.com/sirupsen/logrus"
)

var errClosing = errors.New("the db is in closing state")
var errOutOfSpace = errors.New("running out of space")

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
	err := os.MkdirAll(badgerPath, 0o755)
	if err != nil {
		return nil, err
	}
	badgerOptions := badger.DefaultOptions(badgerPath)
	badgerOptions = badgerOptions.WithTruncate(!cfg.Server.BadgerNoTruncate)
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
		return segment.FromBytes(v)
	}
	s.segments.New = func(_k string) interface{} {
		return segment.New()
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

	// load the total memory of the server
	vm, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}

	interval := time.Duration(10)
	// start a timer for checking if the memory used by application
	// is more than 25% of the total memory, if so, trigger eviction
	// with 10% to every cache
	go func() {
		ticker := time.NewTimer(interval * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			// read the allocated memory used by application
			var m runtime.MemStats
			runtime.ReadMemStats(&m)

			used := float64(m.Alloc) / float64(vm.Total)

			logrus.Infof("current used percent of memory: %.5f, %f, %f", used, s.cfg.Server.CacheEviction, s.cfg.Server.CacheEvictionPercent)
			if used > s.cfg.Server.CacheEviction {
				percent := s.cfg.Server.CacheEvictionPercent

				s.dimensions.Evit(percent)
				s.segments.Evit(percent)
				s.trees.Evit(percent)
				s.dicts.Evit(percent)
			}

			// reset the timer
			ticker.Reset(interval * time.Second)
		}
	}()

	return s, nil
}

type PutInput struct {
	StartTime       time.Time
	EndTime         time.Time
	Key             *Key
	Val             *tree.Tree
	SpyName         string
	SampleRate      uint32
	Units           string
	AggregationType string
}

func (s *Storage) Put(po *PutInput) error {
	s.closingMutex.Lock()
	defer s.closingMutex.Unlock()

	if s.closing {
		return errClosing
	}

	freeSpace, err := disk.FreeSpace(s.cfg.Server.StoragePath)
	if err == nil && freeSpace < s.cfg.Server.OutOfSpaceThreshold {
		return errOutOfSpace
	}

	logrus.WithFields(logrus.Fields{
		"startTime":       po.StartTime.String(),
		"endTime":         po.EndTime.String(),
		"key":             po.Key.Normalized(),
		"samples":         po.Val.Samples(),
		"units":           po.Units,
		"aggregationType": po.AggregationType,
	}).Info("storage.Put")
	for k, v := range po.Key.labels {
		s.labels.Put(k, v)
	}

	sk := po.Key.SegmentKey()
	for k, v := range po.Key.labels {
		d := s.dimensions.Get(k + ":" + v).(*dimension.Dimension)
		d.Insert([]byte(sk))
	}

	st := s.segments.Get(sk).(*segment.Segment)
	st.SetMetadata(po.SpyName, po.SampleRate, po.Units, po.AggregationType)
	samples := po.Val.Samples()
	st.Put(po.StartTime, po.EndTime, samples, func(depth int, t time.Time, r *big.Rat, addons []segment.Addon) {
		tk := po.Key.TreeKey(depth, t)
		existingTree := s.trees.Get(tk).(*tree.Tree)
		treeClone := po.Val.Clone(r)
		for _, addon := range addons {
			tk2 := po.Key.TreeKey(addon.Depth, addon.T)
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

type GetInput struct {
	StartTime time.Time
	EndTime   time.Time
	Key       *Key
}

type GetOutput struct {
	Tree       *tree.Tree
	Timeline   *segment.Timeline
	SpyName    string
	SampleRate uint32
	Units      string
}

func (s *Storage) Get(gi *GetInput) (*GetOutput, error) {
	s.closingMutex.Lock()
	defer s.closingMutex.Unlock()

	if s.closing {
		return nil, errClosing
	}

	logrus.WithFields(logrus.Fields{
		"startTime": gi.StartTime.String(),
		"endTime":   gi.EndTime.String(),
		"key":       gi.Key.Normalized(),
	}).Info("storage.Get")
	triesToMerge := []merge.Merger{}

	dimensions := []*dimension.Dimension{}
	for k, v := range gi.Key.labels {
		d := s.dimensions.Get(k + ":" + v).(*dimension.Dimension)
		dimensions = append(dimensions, d)
	}

	segmentKeys := dimension.Intersection(dimensions...)

	tl := segment.GenerateTimeline(gi.StartTime, gi.EndTime)
	var lastSegment *segment.Segment
	var writesTotal uint64
	aggregationType := "sum"
	for _, sk := range segmentKeys {
		// TODO: refactor, store `Key`s in dimensions
		skk, _ := ParseKey(string(sk))
		st := s.segments.Get(skk.SegmentKey()).(*segment.Segment)
		if st == nil {
			continue
		}

		if st.AggregationType() == "average" {
			aggregationType = "average"
		}

		lastSegment = st

		tl.PopulateTimeline(st)

		st.Get(gi.StartTime, gi.EndTime, func(depth int, samples, writes uint64, t time.Time, r *big.Rat) {
			k := skk.TreeKey(depth, t)
			tr := s.trees.Get(k).(*tree.Tree)
			// TODO: these clones are probably are not the most efficient way of doing this
			//   instead this info should be passed to the merger function imo
			tr2 := tr.Clone(r)
			triesToMerge = append(triesToMerge, merge.Merger(tr2))
			writesTotal += writes
		})
	}

	resultTrie := merge.MergeTriesConcurrently(runtime.NumCPU(), triesToMerge...)
	if resultTrie == nil {
		return nil, nil
	}

	t := resultTrie.(*tree.Tree)

	if writesTotal > 0 && aggregationType == "average" {
		t = t.Clone(big.NewRat(1, int64(writesTotal)))
	}

	return &GetOutput{
		Tree:       t,
		Timeline:   tl,
		SpyName:    lastSegment.SpyName(),
		SampleRate: lastSegment.SampleRate(),
		Units:      lastSegment.Units(),
	}, nil
}

func (s *Storage) Close() error {
	s.closingMutex.Lock()
	s.closing = true
	s.closingMutex.Unlock()

	wg := sync.WaitGroup{}
	wg.Add(3)
	go func() { defer wg.Done(); s.dimensions.Flush() }()
	go func() { defer wg.Done(); s.segments.Flush() }()
	go func() { defer wg.Done(); s.trees.Flush() }()
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
	s.labels.GetValues(key, func(v string) bool {
		if key != "__name__" || !slices.StringContains(s.cfg.Server.HideApplications, v) {
			return cb(v)
		}
		return true
	})
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
