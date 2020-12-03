package storage

import (
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	"github.com/petethepig/pyroscope/pkg/config"
	"github.com/petethepig/pyroscope/pkg/storage/cache"
	"github.com/petethepig/pyroscope/pkg/storage/dict"
	"github.com/petethepig/pyroscope/pkg/storage/labels"
	"github.com/petethepig/pyroscope/pkg/storage/segment"
	"github.com/petethepig/pyroscope/pkg/storage/tree"
	"github.com/petethepig/pyroscope/pkg/structs/merge"
	"github.com/petethepig/pyroscope/pkg/timing"
	"github.com/spaolacci/murmur3"
)

type Storage struct {
	cfg      *config.Config
	segments *cache.Cache

	dicts  *cache.Cache
	trees  *cache.Cache
	labels *labels.Labels

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

func treeKey(normalizedLabelsString string, depth int, t time.Time) string {
	u1, u2 := murmur3.Sum128WithSeed([]byte(normalizedLabelsString), 6231912)

	b := make([]byte, 32)
	binary.LittleEndian.PutUint64(b[:8], u1)
	binary.LittleEndian.PutUint64(b[8:16], u2)
	binary.BigEndian.PutUint64(b[16:24], uint64(depth))
	binary.BigEndian.PutUint64(b[24:32], uint64(t.Unix()))
	b2 := make([]byte, 64)
	hex.Encode(b2, b)
	return string(b2)
}

func (s *Storage) Put(startTime, endTime time.Time, key string, val *tree.Tree) (*timing.Timer, error) {
	timer := timing.New()

	for _, pair := range strings.Split(key, ";") {
		arr := strings.Split(pair, "=")
		if len(arr) == 2 {
			s.labels.Put(arr[0], arr[1])
		}
	}

	sk := segment.Key(key)
	st := s.segments.Get(string(sk)).(*segment.Segment)
	st.Put(startTime, endTime, func(depth int, t time.Time, m, d int) {
		tk := treeKey(key, depth, t)
		existingTree := s.trees.Get(tk).(*tree.Tree)
		val = val.Clone(m, d)
		if existingTree != nil {
			existingTree.Merge(val)
			s.trees.Put(tk, existingTree)
		} else {
			s.trees.Put(tk, val)
		}
	})
	s.segments.Put(string(sk), st)

	return timer, nil
}

func (s *Storage) Get(startTime, endTime time.Time, key string) (*tree.Tree, error) {
	triesToMerge := []merge.Merger{}
	sk := segment.Key(key)
	st := s.segments.Get(string(sk)).(*segment.Segment)
	if st == nil {
		return nil, nil
	}

	st.Get(startTime, endTime, func(d int, t time.Time) {
		k := treeKey(key, d, t)
		tr := s.trees.Get(k).(*tree.Tree)
		triesToMerge = append(triesToMerge, merge.Merger(tr))
	})
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
	wg.Add(2)
	go func() { s.segments.Flush(); wg.Done() }()
	go func() { s.trees.Flush(); wg.Done() }()
	wg.Wait()
	// dictionary has to flush last because trees write to dictionaries
	s.dicts.Flush()
	s.db.Close()
}
