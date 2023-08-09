package symdb

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/pprof/profile"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
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

type Stats struct {
	StacktracesTotal int
	MaxStacktraceID  int
	LocationsTotal   int
	MappingsTotal    int
	FunctionsTotal   int
	StringsTotal     int
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
		header:      PartitionHeader{Partition: partition},
		stacktraces: newStacktracesPartition(s.config.Stacktraces.MaxNodesPerChunk),
	}
	p.strings.init()
	p.mappings.init()
	p.functions.init()
	p.locations.init()
	return &p
}

func (s *SymDB) WriteProfileSymbols(partition uint64, profile *profilev1.Profile) []schemav1.InMemoryProfile {
	return s.SymbolsWriter(partition).WriteProfileSymbols(profile)
}

func (s *SymDB) ResolveTree(ctx context.Context, m schemav1.SampleMap) (*phlaremodel.Tree, error) {
	return ResolveTree(ctx, m, defaultResolveConcurrency, s.withResolver)
}

func (s *SymDB) ResolveProfile(ctx context.Context, m schemav1.SampleMap) (*profile.Profile, error) {
	return ResolveProfile(ctx, m, defaultResolveConcurrency, s.withResolver)
}

func (s *SymDB) withResolver(_ context.Context, partition uint64, fn func(*Resolver) error) error {
	pr, err := s.SymbolsReader(partition)
	if err != nil {
		return err
	}
	return fn(pr.Resolver())
}

func (s *SymDB) SymbolsReader(partition uint64) (*Partition, error) {
	if p, ok := s.lookupPartition(partition); ok {
		return p, nil
	}
	return nil, ErrPartitionNotFound
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
	partitions := make([]*Partition, len(s.partitions))
	var i int
	for _, v := range s.partitions {
		partitions[i] = v
		i++
	}
	sort.Slice(partitions, func(i, j int) bool {
		return partitions[i].header.Partition < partitions[j].header.Partition
	})
	if err := s.writer.createDir(); err != nil {
		return err
	}
	if err := s.writer.WritePartitions(partitions); err != nil {
		return fmt.Errorf("writing partitions: %w", err)
	}
	return s.writer.Flush()
}

func (s *SymDB) Files() []block.File {
	return s.writer.files
}
