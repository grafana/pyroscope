package symdb

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type SymDB struct {
	config *Config
	writer *Writer
	stats  stats

	m        sync.RWMutex
	mappings map[uint64]*inMemoryMapping

	wg   sync.WaitGroup
	stop chan struct{}
}

type Config struct {
	Dir         string
	Stacktraces StacktracesConfig
}

type StacktracesConfig struct {
	MaxNodesPerChunk uint32
}

const statsUpdateInterval = 10 * time.Second

type stats struct {
	memorySize atomic.Uint64
	mappings   atomic.Uint32
}

func DefaultConfig() *Config {
	return &Config{
		Dir: DefaultDirName,
		Stacktraces: StacktracesConfig{
			// A million of nodes ensures predictable
			// memory consumption, although causes a
			// small overhead.
			MaxNodesPerChunk: 1 << 20,
		},
	}
}

func (c *Config) WithDirectory(dir string) *Config {
	c.Dir = dir
	return c
}

func NewSymDB(c *Config) *SymDB {
	if c == nil {
		c = DefaultConfig()
	}
	db := &SymDB{
		config:   c,
		writer:   NewWriter(c.Dir),
		mappings: make(map[uint64]*inMemoryMapping),
		stop:     make(chan struct{}),
	}
	db.wg.Add(1)
	go db.updateStats()
	return db
}

func (s *SymDB) MappingWriter(mappingName uint64) MappingWriter {
	return s.mapping(mappingName)
}

func (s *SymDB) MappingReader(mappingName uint64) (MappingReader, bool) {
	return s.lookupMapping(mappingName)
}

func (s *SymDB) lookupMapping(mappingName uint64) (*inMemoryMapping, bool) {
	s.m.RLock()
	p, ok := s.mappings[mappingName]
	if ok {
		s.m.RUnlock()
		return p, true
	}
	s.m.RUnlock()
	return nil, false
}

func (s *SymDB) mapping(mappingName uint64) *inMemoryMapping {
	p, ok := s.lookupMapping(mappingName)
	if ok {
		return p
	}
	s.m.Lock()
	if p, ok = s.mappings[mappingName]; ok {
		s.m.Unlock()
		return p
	}
	p = &inMemoryMapping{
		name:               mappingName,
		maxNodesPerChunk:   s.config.Stacktraces.MaxNodesPerChunk,
		stacktraceHashToID: make(map[uint64]uint32, defaultStacktraceTreeSize/2),
	}
	p.stacktraceChunks = append(p.stacktraceChunks, &stacktraceChunk{
		tree:    newStacktraceTree(defaultStacktraceTreeSize),
		mapping: p,
	})
	s.mappings[mappingName] = p
	s.m.Unlock()
	return p
}

func (s *SymDB) Flush() error {
	close(s.stop)
	s.wg.Wait()
	s.m.RLock()
	m := make([]*inMemoryMapping, len(s.mappings))
	var i int
	for _, v := range s.mappings {
		m[i] = v
		i++
	}
	s.m.RUnlock()
	sort.Slice(m, func(i, j int) bool {
		return m[i].name < m[j].name
	})
	for _, v := range m {
		for ci, c := range v.stacktraceChunks {
			if err := s.writer.writeStacktraceChunk(ci, c); err != nil {
				return err
			}
		}
	}
	return s.writer.Flush()
}

func (s *SymDB) Name() string { return s.config.Dir }

func (s *SymDB) Size() uint64 {
	// NOTE(kolesnikovae): SymDB does not use disk until flushed.
	//  This method should be implemented once the logic changes.
	return 0
}

func (s *SymDB) MemorySize() uint64 { return s.stats.memorySize.Load() }

func (s *SymDB) updateStats() {
	t := time.NewTicker(statsUpdateInterval)
	defer func() {
		t.Stop()
		s.wg.Done()
	}()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			s.m.RLock()
			s.stats.mappings.Store(uint32(len(s.mappings)))
			s.stats.memorySize.Store(uint64(s.calculateMemoryFootprint()))
			s.m.RUnlock()
		}
	}
}

// calculateMemoryFootprint estimates the memory footprint.
func (s *SymDB) calculateMemoryFootprint() (v int) {
	for _, m := range s.mappings {
		m.stacktraceMutex.RLock()
		v += len(m.stacktraceChunkHeaders) * stacktraceChunkHeaderSize
		for _, c := range m.stacktraceChunks {
			v += stacktraceTreeNodeSize * cap(c.tree.nodes)
		}
		m.stacktraceMutex.RUnlock()
	}
	return v
}
