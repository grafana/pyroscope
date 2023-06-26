package symdb

import (
	"sort"
	"sync"
)

type SymDB struct {
	config *Config
	writer *Writer

	m        sync.RWMutex
	mappings map[uint64]*inMemoryMapping
}

type Config struct {
	Dir         string
	Stacktraces StacktracesConfig
}

type StacktracesConfig struct {
	MaxNodesPerChunk uint32
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
	return &SymDB{
		config:   c,
		writer:   NewWriter(c.Dir),
		mappings: make(map[uint64]*inMemoryMapping),
	}
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

// TODO(kolesnikovae): Implement:

type Stats struct {
	MemorySize uint64
	Mappings   uint32
}

func (s *SymDB) Stats() Stats {
	return Stats{}
}

// TODO(kolesnikovae): Follow Table interface (but Init method).

func (s *SymDB) Name() string { return s.config.Dir }

func (s *SymDB) Size() uint64 { return 0 }

func (s *SymDB) MemorySize() uint64 { return 0 }

func (s *SymDB) Flush() error {
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
