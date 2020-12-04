package storage

import (
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	"github.com/petethepig/pyroscope/pkg/config"
	"github.com/petethepig/pyroscope/pkg/storage/cache"
	"github.com/petethepig/pyroscope/pkg/storage/dict"
	"github.com/petethepig/pyroscope/pkg/storage/dimension"
	"github.com/petethepig/pyroscope/pkg/storage/labels"
	"github.com/petethepig/pyroscope/pkg/storage/segment"
	"github.com/petethepig/pyroscope/pkg/storage/tree"
	"github.com/petethepig/pyroscope/pkg/structs/merge"
	"github.com/petethepig/pyroscope/pkg/timing"
	"github.com/sirupsen/logrus"
)

type Storage struct {
	cfg      *config.Config
	segments *cache.Cache

	dimensions *cache.Cache
	dicts      *cache.Cache
	trees      *cache.Cache
	labels     *labels.Labels

	db *badger.DB
}

func New(cfg *config.Config) (*Storage, error) {
	// spew.Dump(cfg)
	badgerPath := filepath.Join(cfg.Server.StoragePath, "badger")
	err := os.MkdirAll(badgerPath, 0755)
	if err != nil {
		return nil, err
	}
	badgerOptions := badger.DefaultOptions(badgerPath)
	badgerOptions = badgerOptions.WithTruncate(true)
	badgerOptions = badgerOptions.WithCompression(options.ZSTD)
	badgerOptions = badgerOptions.WithLogger(badgerLogger{})
	// badgerOptions = badgerOptions.WithSyncWrites(true)
	db, err := badger.Open(badgerOptions)
	if err != nil {
		return nil, err
	}

	s := &Storage{
		cfg:    cfg,
		labels: labels.New(cfg, db),
		db:     db,
	}

	s.dimensions = cache.New(db, cfg.Server.CacheSegmentSize, "d:")
	s.dimensions.Bytes = func(v interface{}) []byte {
		return v.(*dimension.Dimension).Bytes()
	}
	s.dimensions.FromBytes = func(v []byte) interface{} {
		return dimension.FromBytes(v)
	}
	s.dimensions.New = func() interface{} {
		return dimension.New()
	}

	s.segments = cache.New(db, cfg.Server.CacheSegmentSize, "s:")
	s.segments.Bytes = func(v interface{}) []byte {
		return v.(*segment.Segment).Bytes()
	}
	s.segments.FromBytes = func(v []byte) interface{} {
		return segment.FromBytes(cfg.Server.MinResolution, cfg.Server.Multiplier, v)
	}
	s.segments.New = func() interface{} {
		return segment.New(s.cfg.Server.MinResolution, s.cfg.Server.Multiplier)
	}

	s.dicts = cache.New(db, cfg.Server.CacheSegmentSize, "d:")
	s.dicts.Bytes = func(v interface{}) []byte {
		return v.(*dict.Dict).Bytes()
	}
	s.dicts.FromBytes = func(v []byte) interface{} {
		return dict.FromBytes(v)
	}
	s.dicts.New = func() interface{} {
		return dict.New()
	}

	// for now there's just one Dict, I think going forward we could have different ones for different
	//   types of profiles
	d := s.dicts.Get("main-dict").(*dict.Dict)

	s.trees = cache.New(db, cfg.Server.CacheSegmentSize, "t:")
	s.trees.Bytes = func(v interface{}) []byte {
		return v.(*tree.Tree).Bytes(d)
	}
	s.trees.FromBytes = func(v []byte) interface{} {
		return tree.FromBytes(d, v)
	}
	s.trees.New = func() interface{} {
		return tree.New()
	}

	return s, nil
}

func treeKey(sk segment.Key, depth int, t time.Time) string {
	b := make([]byte, 32)
	copy(b[:16], sk)
	binary.BigEndian.PutUint64(b[16:24], uint64(depth))
	binary.BigEndian.PutUint64(b[24:32], uint64(t.Unix()))
	b2 := make([]byte, 64)
	hex.Encode(b2, b)
	return string(b2)
}

func (s *Storage) Put(startTime, endTime time.Time, key *Key, val *tree.Tree) (*timing.Timer, error) {
	timer := timing.New()

	for k, v := range key.labels {
		s.labels.Put(k, v)
	}

	sk := segment.Key(key.Normalized())

	for k, v := range key.labels {
		d := s.dimensions.Get(k + ":" + v).(*dimension.Dimension)
		d.Insert(sk)
	}

	st := s.segments.Get(string(sk)).(*segment.Segment)
	st.Put(startTime, endTime, func(depth int, t time.Time, m, d int) {
		tk := treeKey(sk, depth, t)
		existingTree := s.trees.Get(tk).(*tree.Tree)
		treeClone = val.Clone(m, d)
		if existingTree != nil {
			existingTree.Merge(treeClone)
			s.trees.Put(tk, existingTree)
		} else {
			s.trees.Put(tk, treeClone)
		}
	})
	s.segments.Put(string(sk), st)

	return timer, nil
}

func (s *Storage) Get(startTime, endTime time.Time, key *Key) (*tree.Tree, error) {
	triesToMerge := []merge.Merger{}

	dimensions := []*dimension.Dimension{}
	for k, v := range key.labels {
		d := s.dimensions.Get(k + ":" + v).(*dimension.Dimension)
		logrus.Debugf("keys: %q %q %q", k, v, d.Bytes())
		dimensions = append(dimensions, d)
	}

	segmentKeys := dimension.Intersection(dimensions...)

	for _, sk := range segmentKeys {
		logrus.Debug("sk", sk)
		st := s.segments.Get(string(sk)).(*segment.Segment)
		if st == nil {
			continue
		}

		st.Get(startTime, endTime, func(d int, t time.Time) {
			k := treeKey(sk, d, t)
			tr := s.trees.Get(k).(*tree.Tree)
			triesToMerge = append(triesToMerge, merge.Merger(tr))
		})
	}

	resultTrie := merge.MergeTriesConcurrently(runtime.NumCPU(), triesToMerge...)
	if resultTrie == nil {
		return nil, nil
	}
	return resultTrie.(*tree.Tree), nil
}

func (s *Storage) Close() error {
	s.Cleanup()
	return s.db.Close()
}

func (s *Storage) GetKeys(cb func(k string) bool) {
	s.labels.GetKeys(cb)
}

func (s *Storage) GetValues(key string, cb func(v string) bool) {
	s.labels.GetValues(key, cb)
}

func (s *Storage) Cleanup() {
	wg := sync.WaitGroup{}
	wg.Add(3)
	go func() { s.dimensions.Flush(); wg.Done() }()
	go func() { s.segments.Flush(); wg.Done() }()
	go func() { s.trees.Flush(); wg.Done() }()
	wg.Wait()
	// dictionary has to flush last because trees write to dictionaries
	s.dicts.Flush()
	s.db.Close()
	time.Sleep(5 * time.Second)
}
