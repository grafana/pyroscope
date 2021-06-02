package storage

import (
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/util/disk"

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

	cfg      *config.Server
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

func newBadger(cfg *config.Server, name string) (*badger.DB, error) {
	badgerPath := filepath.Join(cfg.StoragePath, name)
	err := os.MkdirAll(badgerPath, 0o755)
	if err != nil {
		return nil, err
	}
	badgerOptions := badger.DefaultOptions(badgerPath)
	badgerOptions = badgerOptions.WithTruncate(!cfg.BadgerNoTruncate)
	badgerOptions = badgerOptions.WithSyncWrites(false)
	badgerOptions = badgerOptions.WithCompression(options.ZSTD)
	badgerLevel := logrus.ErrorLevel
	if l, err := logrus.ParseLevel(cfg.BadgerLogLevel); err == nil {
		badgerLevel = l
	}
	badgerOptions = badgerOptions.WithLogger(badgerLogger{name: name, logLevel: badgerLevel})

	db, err := badger.Open(badgerOptions)
	if err == nil {
		go badgerGC(db)
	}
	return db, err
}

func New(cfg *config.Server) (*Storage, error) { // TODO: cfg.Server?
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
		labels:       labels.New(db),
		db:           db,
		dbTrees:      dbTrees,
		dbDicts:      dbDicts,
		dbDimensions: dbDimensions,
		dbSegments:   dbSegments,
	}

	s.dimensions = cache.New(dbDimensions, cfg.CacheDimensionSize, "i:")
	s.dimensions.Bytes = func(k string, v interface{}) ([]byte, error) {
		return v.(*dimension.Dimension).Bytes()
	}
	s.dimensions.FromBytes = func(k string, v []byte) (interface{}, error) {
		return dimension.FromBytes(v)
	}
	s.dimensions.New = func(k string) interface{} {
		return dimension.New()
	}

	s.segments = cache.New(dbSegments, cfg.CacheSegmentSize, "s:")
	s.segments.Bytes = func(k string, v interface{}) ([]byte, error) {
		return v.(*segment.Segment).Bytes()
	}
	s.segments.FromBytes = func(k string, v []byte) (interface{}, error) {
		// TODO:
		//   these configuration params should be saved in db when it initializes
		return segment.FromBytes(v)
	}
	s.segments.New = func(k string) interface{} {
		return segment.New()
	}

	s.dicts = cache.New(dbDicts, cfg.CacheDictionarySize, "d:")
	s.dicts.Bytes = func(k string, v interface{}) ([]byte, error) {
		return v.(*dict.Dict).Bytes()
	}
	s.dicts.FromBytes = func(k string, v []byte) (interface{}, error) {
		return dict.FromBytes(v)
	}
	s.dicts.New = func(k string) interface{} {
		return dict.New()
	}

	s.trees = cache.New(dbTrees, cfg.CacheTreeSize, "t:")
	s.trees.Bytes = func(k string, v interface{}) ([]byte, error) {
		key := FromTreeToMainKey(k)
		d, err := s.dicts.Get(key)
		if err != nil {
			return nil, fmt.Errorf("dicts cache for %v: %v", key, err)
		}
		if d == nil { // key not found
			return nil, nil
		}
		return v.(*tree.Tree).Bytes(d.(*dict.Dict), cfg.MaxNodesSerialization)
	}
	s.trees.FromBytes = func(k string, v []byte) (interface{}, error) {
		key := FromTreeToMainKey(k)
		d, err := s.dicts.Get(FromTreeToMainKey(k))
		if err != nil {
			return nil, fmt.Errorf("dicts cache for %v: %v", key, err)
		}
		if d == nil { // key not found
			return nil, nil
		}
		return tree.FromBytes(d.(*dict.Dict), v)
	}
	s.trees.New = func(k string) interface{} {
		return tree.New()
	}

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

	freeSpace, err := disk.FreeSpace(s.cfg.StoragePath)
	if err == nil && freeSpace < s.cfg.OutOfSpaceThreshold {
		if s.cfg.ThresholdModeAuto {
			// TODO: Threshold calculation & Handle race condition
			logrus.Debugf("Triggered auto cleanup")
			// defer s.Cleanup()
		}
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
		key := k + ":" + v
		res, err := s.dimensions.Get(key)
		if err != nil {
			logrus.Errorf("dimensions cache for %v: %v", key, err)
			continue
		}
		if res != nil {
			res.(*dimension.Dimension).Insert([]byte(sk))
		}
	}

	res, err := s.segments.Get(sk)
	if err != nil {
		return fmt.Errorf("segments cache for %v: %v", sk, err)
	}
	if res == nil {
		return fmt.Errorf("segments cache for %v: not found", sk)
	}

	st := res.(*segment.Segment)
	st.SetMetadata(po.SpyName, po.SampleRate, po.Units, po.AggregationType)
	samples := po.Val.Samples()
	st.Put(po.StartTime, po.EndTime, samples, func(depth int, t time.Time, r *big.Rat, addons []segment.Addon) {
		tk := po.Key.TreeKey(depth, t)

		res, err := s.trees.Get(tk)
		if err != nil {
			logrus.Errorf("trees cache for %v: %v", tk, err)
			return
		}
		cachedTree := res.(*tree.Tree)

		treeClone := po.Val.Clone(r)
		for _, addon := range addons {
			tk2 := po.Key.TreeKey(addon.Depth, addon.T)

			res, err := s.trees.Get(tk2)
			if err != nil {
				logrus.Errorf("trees cache for %v: %v", tk, err)
				continue
			}
			if res == nil {
				continue
			}
			treeClone.Merge(res.(*tree.Tree))
		}
		if cachedTree != nil {
			cachedTree.Merge(treeClone)
			s.trees.Put(tk, cachedTree)
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
		key := k + ":" + v
		res, err := s.dimensions.Get(key)
		if err != nil {
			logrus.Errorf("dimensions cache for %v: %v", key, err)
			continue
		}
		if res != nil {
			dimensions = append(dimensions, res.(*dimension.Dimension))
		}
	}

	segmentKeys := dimension.Intersection(dimensions...)

	tl := segment.GenerateTimeline(gi.StartTime, gi.EndTime)
	var lastSegment *segment.Segment
	var writesTotal uint64
	aggregationType := "sum"
	for _, sk := range segmentKeys {
		// TODO: refactor, store `Key`s in dimensions
		parsedKey, err := ParseKey(string(sk))
		if err != nil {
			logrus.Errorf("parse key: %v: %v", string(sk), err)
			continue
		}

		key := parsedKey.SegmentKey()
		res, err := s.segments.Get(key)
		if err != nil {
			logrus.Errorf("segments cache for %v: %v", key, err)
			continue
		}
		if res == nil {
			continue
		}

		st := res.(*segment.Segment)
		if st.AggregationType() == "average" {
			aggregationType = "average"
		}
		lastSegment = st

		tl.PopulateTimeline(st)

		st.Get(gi.StartTime, gi.EndTime, func(depth int, samples, writes uint64, t time.Time, r *big.Rat) {
			key := parsedKey.TreeKey(depth, t)
			res, err := s.trees.Get(key)
			if err != nil {
				logrus.Errorf("trees cache for %v: %v", key, err)
				return
			}

			tr := res.(*tree.Tree)
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

func (s *Storage) Cleanup() error {
	s.closingMutex.Lock()
	defer s.closingMutex.Unlock()

	if s.closing {
		return errClosing
	}

	nameKey := "__name__"

	lg := logrus.WithField("task", "cleanup")

	var dimensions []*dimension.Dimension
	var err error
	s.labels.GetValues(nameKey, func(v string) bool {
		dmInt, getErr := s.dimensions.Get(nameKey + ":" + v)
		dm, _ := dmInt.(*dimension.Dimension)
		err = getErr
		dimensions = append(dimensions, dm)
		return err == nil
	})

	if err != nil {
		return err
	}

	logrus.Debugf("Dimension len: %d", len(dimensions))

	segmentKeys := dimension.Union(dimensions...)

	logrus.Debugf("Segment key count: %d", len(segmentKeys))
	for _, rawSk := range segmentKeys {
		sk, _ := ParseKey(string(rawSk))
		logrus.Debugf("Segment key: %s", sk)

		stInt, err := s.segments.Get(sk.SegmentKey())
		if err != nil {
			return err
		}
		st := stInt.(*segment.Segment)
		hasData := st.Cleanup(time.Now().UTC().Add(s.cfg.RetentionThreshold), func(depth int, t time.Time) {
			tk := sk.TreeKey(depth, t)

			logrus.Debugf("Tree key: %s", tk)
			if err := s.trees.Delete(tk); err != nil {
				lg.Errorf("%v", err)
			}
		})

		if !hasData {
			if err := s.segments.Delete(sk.SegmentKey()); err != nil {
				lg.Errorf("%v", err)
			}
		}

		lg.Debugf("sk: %s", sk.SegmentKey())
	}

	lg.Debugf("segment cleanup completed")
	return nil
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
	s.labels.GetValues(key, func(v string) bool {
		if key != "__name__" || !slices.StringContains(s.cfg.HideApplications, v) {
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
		res[k] = dirSize(filepath.Join(s.cfg.StoragePath, k))
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

func (s *Storage) CacheStats() map[string]interface{} {
	return map[string]interface{}{
		"dimensions": s.dimensions.Size(),
		"segments":   s.segments.Size(),
		"dicts":      s.dicts.Size(),
		"trees":      s.trees.Size(),
	}
}
