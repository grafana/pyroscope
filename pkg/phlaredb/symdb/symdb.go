package symdb

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

// SymbolsReader provides access to a symdb partition.
type SymbolsReader interface {
	Partition(ctx context.Context, partition uint64) (PartitionReader, error)
}

type PartitionReader interface {
	WriteStats(s *PartitionStats)
	Symbols() *Symbols
	Release()
}

type Symbols struct {
	Stacktraces StacktraceResolver
	Locations   []schemav1.InMemoryLocation
	Mappings    []schemav1.InMemoryMapping
	Functions   []schemav1.InMemoryFunction
	Strings     []string
}

type PartitionStats struct {
	StacktracesTotal int
	MaxStacktraceID  int
	LocationsTotal   int
	MappingsTotal    int
	FunctionsTotal   int
	StringsTotal     int
}

type StacktraceResolver interface {
	// ResolveStacktraceLocations resolves locations for each stack
	// trace and inserts it to the StacktraceInserter provided.
	//
	// The stacktraces must be ordered in the ascending order.
	// If a stacktrace can't be resolved, dst receives an empty
	// array of locations.
	//
	// Stacktraces slice might be modified during the call.
	ResolveStacktraceLocations(ctx context.Context, dst StacktraceInserter, stacktraces []uint32) error
	LookupLocations(dst []uint64, stacktraceID uint32) []uint64

	// Optional:
	// StacktraceIDRangeIterator
}

// StacktraceIDRangeIterator provides low level access
// to stack traces, stored in painter point trees.
type StacktraceIDRangeIterator interface {
	SplitStacktraceIDRanges(*SampleAppender) iter.Iterator[*StacktraceIDRange]
}

type ParentPointerTree interface {
	Nodes() []Node
}

type Node struct {
	Parent   int32
	Location int32
	Value    int64
}

// StacktraceInserter accepts resolved locations for a given stack
// trace. The leaf is at locations[0].
//
// Locations slice must not be retained by implementation.
// It is guaranteed, that for a given stacktrace ID
// InsertStacktrace is called not more than once.
type StacktraceInserter interface {
	InsertStacktrace(stacktraceID uint32, locations []int32)
}

type SymDB struct {
	config Config
	writer blockWriter
	stats  MemoryStats

	m          sync.RWMutex
	partitions map[uint64]*PartitionWriter

	wg   sync.WaitGroup
	stop chan struct{}
}

type Config struct {
	Dir         string
	Version     FormatVersion
	Stacktraces StacktracesConfig
	Parquet     ParquetConfig
}

type StacktracesConfig struct {
	// DEPRECATED: the parameter is not used and
	// will be removed in the future versions.
	MaxNodesPerChunk uint32
}

type ParquetConfig struct {
	// DEPRECATED: the parameter is not used and
	// will be removed in the future versions.
	MaxBufferRowCount int
}

type MemoryStats struct {
	StacktracesSize uint64
	LocationsSize   uint64
	MappingsSize    uint64
	FunctionsSize   uint64
	StringsSize     uint64
}

func (m *MemoryStats) MemorySize() uint64 {
	return m.StacktracesSize +
		m.LocationsSize +
		m.MappingsSize +
		m.FunctionsSize +
		m.StringsSize
}

const statsUpdateInterval = 5 * time.Second

func DefaultConfig() *Config {
	return &Config{
		Version: FormatV2,
	}
}

func (c *Config) WithDirectory(dir string) *Config {
	c.Dir = dir
	return c
}

func (c *Config) WithVersion(v FormatVersion) *Config {
	c.Version = v
	return c
}

func NewSymDB(c *Config) *SymDB {
	if c == nil {
		c = DefaultConfig()
	}
	c.Parquet.MaxBufferRowCount = math.MaxInt
	c.Stacktraces.MaxNodesPerChunk = math.MaxUint32
	db := &SymDB{
		config:     *c,
		partitions: make(map[uint64]*PartitionWriter),
		stop:       make(chan struct{}),
	}
	switch c.Version {
	case FormatV3:
		db.writer = newWriterV3(c)
	default:
		db.config.Version = FormatV2
		db.writer = newWriterV2(c)
	}
	db.wg.Add(1)
	go db.updateStatsLoop()
	return db
}

func (s *SymDB) PartitionWriter(partition uint64) *PartitionWriter {
	p, ok := s.lookupPartition(partition)
	if ok {
		return p
	}
	s.m.Lock()
	if p, ok = s.partitions[partition]; ok {
		s.m.Unlock()
		return p
	}
	p = NewPartitionWriter(partition, &s.config)
	s.partitions[partition] = p
	s.m.Unlock()
	return p
}

func NewPartitionWriter(partition uint64, config *Config) *PartitionWriter {
	p := PartitionWriter{
		header:      PartitionHeader{Partition: partition},
		stacktraces: newStacktraces(),
	}
	switch config.Version {
	case FormatV2:
		p.header.V2 = new(PartitionHeaderV2)
	case FormatV3:
		p.header.V3 = new(PartitionHeaderV3)
	}
	p.strings.init()
	p.mappings.init()
	p.functions.init()
	p.locations.init()
	// To ensure that the first string is always "".
	p.strings.slice = append(p.strings.slice, "")
	p.strings.lookup[""] = 0
	return &p
}

func (s *SymDB) WriteProfileSymbols(partition uint64, profile *profilev1.Profile) []schemav1.InMemoryProfile {
	return s.PartitionWriter(partition).WriteProfileSymbols(profile)
}

func (s *SymDB) Partition(_ context.Context, partition uint64) (PartitionReader, error) {
	if p, ok := s.lookupPartition(partition); ok {
		return p, nil
	}
	return nil, ErrPartitionNotFound
}

func (s *SymDB) lookupPartition(partition uint64) (*PartitionWriter, bool) {
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

var emptyMemoryStats MemoryStats

func (s *SymDB) WriteMemoryStats(m *MemoryStats) {
	s.m.Lock()
	c := s.stats
	if c == emptyMemoryStats {
		s.updateStats()
		c = s.stats
	}
	s.m.Unlock()
	*m = c
}

func (s *SymDB) updateStatsLoop() {
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
			s.m.Lock()
			s.updateStats()
			s.m.Unlock()
		}
	}
}

func (s *SymDB) updateStats() {
	s.stats = MemoryStats{}
	for _, m := range s.partitions {
		s.stats.StacktracesSize += m.stacktraces.size()
		s.stats.MappingsSize += m.mappings.Size()
		s.stats.FunctionsSize += m.functions.Size()
		s.stats.LocationsSize += m.locations.Size()
		s.stats.StringsSize += m.strings.Size()
	}
}

func (s *SymDB) Flush() error {
	close(s.stop)
	s.wg.Wait()
	s.updateStats()
	partitions := make([]*PartitionWriter, len(s.partitions))
	var i int
	for _, v := range s.partitions {
		partitions[i] = v
		i++
	}
	sort.Slice(partitions, func(i, j int) bool {
		return partitions[i].header.Partition < partitions[j].header.Partition
	})
	return s.writer.writePartitions(partitions)
}

func (s *SymDB) Files() []block.File {
	return s.writer.meta()
}

func (s *SymDB) FormatVersion() FormatVersion {
	return s.config.Version
}
