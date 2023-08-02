package symdb

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

type SymDB struct {
	config *Config
	writer *Writer
	stats  MemoryStats

	m          sync.RWMutex
	partitions map[uint64]*Partition

	wg   sync.WaitGroup
	stop chan struct{}
}

type Config struct {
	Dir         string
	Stacktraces StacktracesConfig
	Parquet     ParquetConfig
}

type StacktracesConfig struct {
	MaxNodesPerChunk uint32
}

type ParquetConfig struct {
	MaxBufferRowCount int
}

type MemoryStats struct {
	StacktracesSize uint64
	LocationsSize   uint64
	MappingsSize    uint64
	FunctionsSize   uint64
	StringsSize     uint64
}

func (m MemoryStats) MemorySize() uint64 {
	return m.StacktracesSize +
		m.LocationsSize +
		m.MappingsSize +
		m.FunctionsSize +
		m.StringsSize
}

const statsUpdateInterval = 10 * time.Second

func DefaultConfig() *Config {
	return &Config{
		Dir: DefaultDirName,
		Stacktraces: StacktracesConfig{
			// A million of nodes ensures predictable
			// memory consumption, although causes a
			// small overhead.
			MaxNodesPerChunk: 1 << 20,
		},
		Parquet: ParquetConfig{
			MaxBufferRowCount: 100 << 10,
		},
	}
}

func (c *Config) WithDirectory(dir string) *Config {
	c.Dir = dir
	return c
}

func (c *Config) WithParquetConfig(pc ParquetConfig) *Config {
	c.Parquet = pc
	return c
}

func NewSymDB(c *Config) *SymDB {
	if c == nil {
		c = DefaultConfig()
	}
	db := &SymDB{
		config:     c,
		writer:     NewWriter(c),
		partitions: make(map[uint64]*Partition),
		stop:       make(chan struct{}),
	}
	db.wg.Add(1)
	go db.updateStats()
	return db
}

func (s *SymDB) SymbolsWriter(partition uint64) *Partition {
	p, ok := s.lookupPartition(partition)
	if ok {
		return p
	}
	s.m.Lock()
	if p, ok = s.partitions[partition]; ok {
		s.m.Unlock()
		return p
	}
	p = s.newPartition(partition)
	s.partitions[partition] = p
	s.m.Unlock()
	return p
}

func (s *SymDB) newPartition(partition uint64) *Partition {
	p := Partition{
		name:        partition,
		stacktraces: newStacktracesPartition(s.config.Stacktraces.MaxNodesPerChunk),
	}
	p.strings.init()
	p.mappings.init()
	p.functions.init()
	p.locations.init()
	return &p
}

func (s *SymDB) SymbolsReader(partition uint64) (*Partition, bool) {
	return s.lookupPartition(partition)
}

func (s *SymDB) lookupPartition(partition uint64) (*Partition, bool) {
	s.m.RLock()
	p, ok := s.partitions[partition]
	if ok {
		s.m.RUnlock()
		return p, true
	}
	s.m.RUnlock()
	return nil, false
}

func (s *SymDB) Name() string { return s.config.Dir }

func (s *SymDB) Size() uint64 {
	// NOTE(kolesnikovae): SymDB does not use disk until flushed.
	//  This method should be implemented once the logic changes.
	return 0
}

func (s *SymDB) MemorySize() uint64 {
	s.m.RLock()
	m := s.stats
	s.m.RUnlock()
	return m.MemorySize()
}

func (s *SymDB) WriteMemoryStats(m *MemoryStats) {
	s.m.RLock()
	c := s.stats
	s.m.RUnlock()
	*m = c
}

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
			s.calculateMemoryFootprint()
			s.m.RUnlock()
		}
	}
}

// calculateMemoryFootprint estimates the memory footprint.
func (s *SymDB) calculateMemoryFootprint() (v int) {
	for _, m := range s.partitions {
		s.stats.StacktracesSize = m.stacktraces.size()
		s.stats.MappingsSize = m.mappings.Size()
		s.stats.FunctionsSize = m.functions.Size()
		s.stats.LocationsSize = m.locations.Size()
		s.stats.StringsSize = m.strings.Size()
	}
	return v
}

func (s *SymDB) Flush() error {
	close(s.stop)
	s.wg.Wait()
	s.m.RLock()
	partitions := make([]*Partition, len(s.partitions))
	var i int
	for _, v := range s.partitions {
		partitions[i] = v
		i++
	}
	s.m.RUnlock()
	sort.Slice(partitions, func(i, j int) bool {
		return partitions[i].name < partitions[j].name
	})

	if err := s.writer.WritePartitions(partitions); err != nil {
		return fmt.Errorf("writing partitions: %w", err)
	}

	return s.writer.Flush()
}
