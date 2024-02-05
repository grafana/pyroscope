// Copyright 2022-2024 The Parca Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package parca

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"sync"
	"syscall"
	"unsafe"

	"github.com/Masterminds/semver/v3"
	"github.com/cilium/ebpf"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/parca-dev/parca-agent/pkg/cpuinfo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
	"golang.org/x/exp/constraints"

	"github.com/parca-dev/runtime-data/pkg/python"
	"github.com/parca-dev/runtime-data/pkg/ruby"

	"github.com/parca-dev/parca-agent/pkg/buildid"
	"github.com/parca-dev/parca-agent/pkg/cache"
	"github.com/parca-dev/parca-agent/pkg/elfreader"
	"github.com/parca-dev/parca-agent/pkg/profile"
	"github.com/parca-dev/parca-agent/pkg/profiler"
	"github.com/parca-dev/parca-agent/pkg/profiler/cpu/bpf"
	bpfprograms "github.com/parca-dev/parca-agent/pkg/profiler/cpu/bpf/programs"
	"github.com/parca-dev/parca-agent/pkg/profiler/pyperf"
	"github.com/parca-dev/parca-agent/pkg/profiler/rbperf"
	"github.com/parca-dev/parca-agent/pkg/runtime"
	"github.com/parca-dev/parca-agent/pkg/stack/unwind"
)

const (
	debugThreadsIDsMapName    = "debug_threads_ids"
	StackCountsMapName        = "stack_counts"
	eventsCountMapName        = "events_count"
	StackTracesMapName        = "stack_traces"
	heapMapName               = "heap"
	symbolIndexStorageMapName = "symbol_index_storage"
	symbolTableMapName        = "symbol_table"
	eventsMapName             = "events"

	// rbperf maps.
	RubyPIDToRubyThreadMapName       = "pid_to_rb_thread"
	RubyVersionSpecificOffsetMapName = "version_specific_offsets"

	// pyperf maps.
	PythonPIDToInterpreterInfoMapName  = "pid_to_interpreter_info"
	PythonVersionSpecificOffsetMapName = "version_specific_offsets"

	UnwindInfoChunksMapName = "unwind_info_chunks"
	UnwindTablesMapName     = "unwind_tables"
	ProcessInfoMapName      = "process_info"
	ProgramsMapName         = "programs"
	PerCPUStatsMapName      = "percpu_stats"

	// With the current compact rows, the max items we can store in the kernels
	// we have tested is 262k per map, which we rounded it down to 250k.
	MaxUnwindShards       = 30         // How many unwind table shards we have.
	maxUnwindTableSize    = 250 * 1000 // Always needs to be sync with MAX_UNWIND_TABLE_SIZE in the BPF program.
	maxMappingsPerProcess = 400        // Always need to be in sync with MAX_MAPPINGS_PER_PROCESS.
	maxUnwindTableChunks  = 30         // Always need to be in sync with MAX_UNWIND_TABLE_CHUNKS.
	maxProcesses          = 5000       // Always need to be in sync with MAX_PROCESSES.

	/*
		TODO: once we generate the bindings automatically, remove this.

		typedef struct mapping {
			u64 load_address;
			u64 begin;
			u64 end;
			u64 executable_id;
			u64 type;
		} mapping_t;

		typedef struct {
			u64 is_jit_compiler;
			u64 len;
			mapping_t mappings[MAX_MAPPINGS_PER_PROCESS];
		} process_info_t;
	*/
	mappingInfoSizeBytes = 8 + 8 + 8 + 8 + (maxMappingsPerProcess * 8 * 5)
	/*
		TODO: once we generate the bindings automatically, remove this.

		typedef struct shard_info {
			u64 low_pc;
			u64 high_pc;
			u64 shard_index;
			u64 low_index;
			u64 high_index;
		} shard_info_t;

		typedef struct stack_unwind_table_shards {
			shard_info_t shards[MAX_UNWIND_TABLE_CHUNKS];
		} stack_unwind_table_shards_t;
	*/
	unwindShardsSizeBytes = maxUnwindTableChunks * 8 * 5
	/*
		typedef struct __attribute__((packed)) {
			u64 pc;
			s16 lr_offset;(if arch == EM_AARCH64)
			u8 cfa_type;
			u8 rbp_type;
			s16 cfa_offset;
			s16 rbp_offset;
		} stack_unwind_row_t;
	*/
	compactUnwindRowSizeBytesX86             = 14
	compactUnwindRowSizeBytesArm64           = 16
	minRoundsBeforeRedoingUnwindInfo         = 5
	minRoundsBeforeRedoingProcessInformation = 5
	MaxCachedProcesses                       = 100_000

	defaultSymbolTableSize = 64000
)

const (
	mappingTypeJITted  = 1
	mappingTypeSpecial = 2
)

const (
	RequestUnwindInformation = 1 << 63
	RequestProcessMappings   = 1 << 62
	RequestRefreshProcInfo   = 1 << 61
)

var (
	ErrMissing                   = errors.New("missing stack trace")
	ErrUnwindFailed              = errors.New("stack ID is 0, probably stack unwinding failed")
	ErrUnrecoverable             = errors.New("unrecoverable error")
	ErrTooManyExecutableMappings = errors.New("too many executable mappings")
	ErrNeedMoreProfilingRounds   = errors.New("not enough profiling rounds with this unwind info")
)

type Modules struct {
	ParcaNativeSpec    *ebpf.CollectionSpec
	ParcaNativeObjects *ParcaNativeObjects
	RubySpec           *ebpf.CollectionSpec
	RubyObjects        *ParcaRubyObjects
	PythonSpec         *ebpf.CollectionSpec
	PythonObjects      *ParcaPythonObjects
	MapReplacements    map[string]*ebpf.Map
}

type Maps struct {
	logger  log.Logger
	metrics *MapsMetrics

	byteOrder binary.ByteOrder

	modules *Modules

	debugPIDs *ebpf.Map

	StackCounts *ebpf.Map
	eventsCount *ebpf.Map
	stackTraces *ebpf.Map
	symbolTable *ebpf.Map

	rubyPIDToThread            *ebpf.Map
	rubyVersionSpecificOffsets *ebpf.Map
	rubyVersionToOffsetIndex   map[string]uint32

	pythonVersionToOffsetIndex map[string]uint32

	// Keeps track of synced process info and interpreter info.
	syncedInterpreters *cache.Cache[int, runtime.Interpreter]

	unwindShards *ebpf.Map
	unwindTables *ebpf.Map
	programs     *ebpf.Map
	processInfo  *ebpf.Map

	// Unwind stuff 🔬
	processCache      *ProcessCache
	mappingInfoMemory profiler.EfficientBuffer

	buildIDMapping map[string]uint64

	// Which shard we are using
	maxUnwindShards           uint64
	shardIndex                uint64
	executableID              uint64
	compactUnwindRowSizeBytes int
	unwindInfoMemory          profiler.EfficientBuffer
	// Account where we are within a shard
	lowIndex  uint64
	highIndex uint64
	// Other stats
	totalEntries       uint64
	uniqueMappings     uint64
	referencedMappings uint64
	// Counters to ensure we don't clear the unwind info too
	// quickly if we run out of shards.
	waitingToResetUnwindInfo              bool
	profilingRoundsWithoutUnwindInfoReset int64
	// Counters to ensure we don't clear the process info too
	// quickly if we run out of space.
	waitingToResetProcessInfo              bool
	profilingRoundsWithoutProcessInfoReset int64

	mutex sync.Mutex
}

func min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

type ProcessCache struct {
	*cache.Cache[int, uint64]
}

func NewProcessCache(reg prometheus.Registerer) *ProcessCache {
	return &ProcessCache{
		cache.NewLRUCache[int, uint64](
			prometheus.WrapRegistererWith(prometheus.Labels{"cache": "cpu_map"}, reg),
			MaxCachedProcesses,
		),
	}
}

type ProfilerModuleType int

const (
	NativeModule ProfilerModuleType = iota
	RbperfModule
	PyperfModule
)

type stackTraceWithLength struct {
	Len   uint64
	Addrs [bpfprograms.StackDepth]uint64
}

func NewMaps(
	logger log.Logger,
	byteOrder binary.ByteOrder,
	arch elf.Machine,
	modules *Modules,
	metrics *MapsMetrics,
	processCache *ProcessCache,
	syncedInterpreters *cache.Cache[int, runtime.Interpreter],
) (*Maps, error) {

	var compactUnwindRowSizeBytes int
	switch arch {
	case elf.EM_AARCH64:
		compactUnwindRowSizeBytes = compactUnwindRowSizeBytesArm64
	case elf.EM_X86_64:
		compactUnwindRowSizeBytes = compactUnwindRowSizeBytesX86
	default:
		level.Error(logger).Log("msg", "unknown architecture", "arch", arch)
	}

	mappingInfoMemory := make([]byte, 0, mappingInfoSizeBytes)
	unwindInfoMemory := make([]byte, maxUnwindTableSize*compactUnwindRowSizeBytes)

	maps := &Maps{
		logger:                     log.With(logger, "component", "bpf_maps"),
		metrics:                    metrics,
		byteOrder:                  byteOrder,
		processCache:               processCache,
		mappingInfoMemory:          mappingInfoMemory,
		compactUnwindRowSizeBytes:  compactUnwindRowSizeBytes,
		unwindInfoMemory:           unwindInfoMemory,
		buildIDMapping:             make(map[string]uint64),
		mutex:                      sync.Mutex{},
		pythonVersionToOffsetIndex: make(map[string]uint32),
		rubyVersionToOffsetIndex:   make(map[string]uint32),
		syncedInterpreters:         syncedInterpreters,
		modules:                    modules,
	}

	if err := maps.resetInFlightBuffer(); err != nil {
		level.Error(logger).Log("msg", "resetInFlightBuffer failed", "err", err)
	}

	return maps, nil
}

func (m *Maps) ReuseMaps() error {
	m.modules.MapReplacements = map[string]*ebpf.Map{
		heapMapName:               m.modules.ParcaNativeObjects.Heap,
		StackCountsMapName:        m.modules.ParcaNativeObjects.StackCounts,
		StackTracesMapName:        m.modules.ParcaNativeObjects.StackTraces,
		symbolIndexStorageMapName: m.modules.ParcaNativeObjects.SymbolIndexStorage,
		symbolTableMapName:        m.modules.ParcaNativeObjects.SymbolTable,
	}

	return nil
}

// Interpreter Information.

func (m *Maps) setRbperfProcessData(pid int, procData rbperf.ProcessData) error {
	if m.modules.RubyObjects == nil {
		return nil
	}

	pidToRbData := m.modules.RubyObjects.ParcaRubyMaps.PidToRbThread

	buf := new(bytes.Buffer)
	sz := int(unsafe.Sizeof(procData))
	buf.Grow(sz)

	err := binary.Write(buf, binary.LittleEndian, &procData)
	if err != nil {
		return fmt.Errorf("write procData to buffer: %w", err)
	}

	pidToRbDataKey := uint32(pid)
	err = pidToRbData.Update(pidToRbDataKey, buf.Bytes()[:sz], ebpf.UpdateAny)
	if err != nil {
		return fmt.Errorf("update map pid_to_rb_thread: %w", err)
	}
	return nil
}

func (m *Maps) setRbperfVersionOffsets(versionOffsets []ruby.VersionOffsets) error {
	if m.modules.RubyObjects == nil {
		return nil
	}
	if len(versionOffsets) == 0 {
		return fmt.Errorf("no version offsets provided")
	}
	versions := m.modules.RubyObjects.ParcaRubyMaps.VersionSpecificOffsets

	buf := new(bytes.Buffer)
	i := uint32(0)
	for _, versionOffset := range versionOffsets {
		sz := int(unsafe.Sizeof(versionOffset))
		buf.Grow(sz)

		err := binary.Write(buf, binary.LittleEndian, &versionOffset)
		if err != nil {
			return fmt.Errorf("write versionOffsets to buffer: %w", err)
		}

		key := i
		err = versions.Update(key, buf.Bytes()[:sz], ebpf.UpdateAny)
		if err != nil {
			return fmt.Errorf("update map version_specific_offsets: %w", err)
		}

		m.rubyVersionToOffsetIndex[fmt.Sprintf("%d.%d.%d", versionOffset.MajorVersion, versionOffset.MinorVersion, versionOffset.PatchVersion)] = i
		i++
		buf.Reset()
	}

	return nil
}

func (m *Maps) setPyperfIntepreterInfo(pid int, interpInfo pyperf.InterpreterInfo) error {
	if m.modules.PythonObjects == nil {
		return nil
	}
	pidToInterpreterInfo := m.modules.PythonObjects.ParcaPythonMaps.PidToInterpreterInfo

	buf := new(bytes.Buffer)
	sz := int(unsafe.Sizeof(interpInfo))
	buf.Grow(sz)

	err := binary.Write(buf, binary.LittleEndian, &interpInfo)
	if err != nil {
		return fmt.Errorf("write interpreter info to buffer: %w", err)
	}

	pidToProcInfoKey := uint32(pid)
	err = pidToInterpreterInfo.Update(pidToProcInfoKey, buf.Bytes()[:sz], ebpf.UpdateAny)
	if err != nil {
		return fmt.Errorf("update map pid_to_interpreter_info: %w", err)
	}
	return nil
}

func (m *Maps) setPyperfVersionOffsets(versionOffsets []python.VersionOffsets) error {
	if m.modules.PythonObjects == nil {
		return nil
	}
	versions := m.modules.PythonObjects.ParcaPythonMaps.VersionSpecificOffsets

	if len(versionOffsets) == 0 {
		return fmt.Errorf("no version offsets provided")
	}

	buf := new(bytes.Buffer)
	i := uint32(0)
	for _, v := range versionOffsets {
		sz := int(unsafe.Sizeof(v))
		buf.Grow(sz)
		err := binary.Write(buf, binary.LittleEndian, &v)
		if err != nil {
			level.Debug(m.logger).Log("msg", "write versionOffsets to buffer", "err", err)
			continue
		}
		key := i
		err = versions.Update(key, buf.Bytes()[:sz], ebpf.UpdateAny)
		if err != nil {
			level.Debug(m.logger).Log("msg", "update map version_specific_offsets", "err", err)
			continue
		}
		m.pythonVersionToOffsetIndex[fmt.Sprintf("%d.%d", v.MajorVersion, v.MinorVersion)] = i
		i++
		buf.Reset()
	}
	return nil
}

func (m *Maps) SetInterpreterData() error {
	if m.modules.RubyObjects == nil && m.modules.PythonObjects == nil {
		return nil
	}
	symbolIndexStorage := m.modules.ParcaNativeObjects.ParcaNativeMaps.SymbolIndexStorage

	key := uint32(0)
	cpus := cpuinfo.NumCPU()

	value := make([]uint32, cpus)
	for i := 0; i < cpus; i++ {
		value[i] = uint32(1)
	}
	err := symbolIndexStorage.Update(key, value, ebpf.UpdateAny)
	if err != nil {
		return fmt.Errorf("update symbol_index_storage map: %w", err)
	}

	if m.modules.RubyObjects != nil {
		versions, err := ruby.GetVersions()
		if err != nil {
			return fmt.Errorf("get ruby versions: %w", err)
		}

		err = m.setRbperfVersionOffsets(versions)
		if err != nil {
			return fmt.Errorf("set rbperf version offsets: %w", err)
		}
	}
	//
	if m.modules.PythonObjects != nil {
		versions, err := python.GetVersions()
		if err != nil {
			return fmt.Errorf("get python versions: %w", err)
		}

		err = m.setPyperfVersionOffsets(versions)
		if err != nil {
			return fmt.Errorf("set pyperf version offsets: %w", err)
		}
	}

	return nil
}

func (m *Maps) UpdateTailCallsMap() error {
	if m.modules.RubyObjects != nil {
		{
			em := m.modules.ParcaNativeObjects.ParcaNativeMaps.Programs
			k := uint32(bpfprograms.RubyEntrypointProgramFD)
			v := m.modules.RubyObjects.ParcaRubyPrograms.UnwindRubyStack
			err := em.Update(k, v, ebpf.UpdateAny)
			if err != nil {
				return fmt.Errorf("update programs map: %w", err)
			}
		}
		{
			em := m.modules.RubyObjects.ParcaRubyMaps.Programs
			k := uint32(bpfprograms.RubyUnwinderProgramFD)
			v := m.modules.RubyObjects.ParcaRubyPrograms.WalkRubyStack
			err := em.Update(k, v, ebpf.UpdateAny)
			if err != nil {
				return fmt.Errorf("update programs map: %w", err)
			}
		}
	}
	if m.modules.PythonObjects != nil {
		{
			em := m.modules.ParcaNativeObjects.ParcaNativeMaps.Programs
			k := uint32(bpfprograms.PythonEntrypointProgramFD)
			v := m.modules.PythonObjects.ParcaPythonPrograms.UnwindPythonStack
			err := em.Update(k, v, ebpf.UpdateAny)
			if err != nil {
				return fmt.Errorf("update programs map: %w", err)
			}
		}
		{
			em := m.modules.PythonObjects.ParcaPythonMaps.Programs
			k := uint32(bpfprograms.PythonUnwinderProgramFD)
			v := m.modules.PythonObjects.ParcaPythonPrograms.WalkPythonStack
			err := em.Update(k, v, ebpf.UpdateAny)
			if err != nil {
				return fmt.Errorf("update programs map: %w", err)
			}
		}
	}

	return nil
}

// Close closes all the resources associated with the maps.
func (m *Maps) Close() error {
	return m.processCache.Close()
}

// AdjustMapSizes updates the amount of unwind shards.
//
// Note: It must be called before `BPFLoadObject()`.
func (m *Maps) AdjustMapSizes(debugEnabled bool, unwindTableShards, eventsBufferSize uint32) error {
	unwindTables := m.modules.ParcaNativeSpec.Maps[UnwindTablesMapName]
	if unwindTables == nil {
		return fmt.Errorf("get unwind tables map: nil")
	}

	// Adjust unwind_tables size.
	unwindTables.MaxEntries = unwindTableShards
	m.maxUnwindShards = uint64(unwindTableShards)

	if m.modules.PythonSpec != nil || m.modules.RubySpec != nil {
		m.modules.ParcaNativeSpec.Maps[symbolTableMapName].MaxEntries = defaultSymbolTableSize
		if m.modules.RubySpec != nil {
			m.modules.RubySpec.Maps[symbolTableMapName].MaxEntries = defaultSymbolTableSize
		}
		if m.modules.PythonSpec != nil {
			m.modules.PythonSpec.Maps[symbolTableMapName].MaxEntries = defaultSymbolTableSize
		}
	}

	// Adjust events size.
	eventCounts := m.modules.ParcaNativeSpec.Maps[eventsMapName]
	if eventCounts == nil {
		return fmt.Errorf("get event map: nil")
	}
	eventCounts.MaxEntries = eventsBufferSize

	// Adjust debug_threads_ids size.
	if debugEnabled {
		//debugThreadsIDs, err := m.nativeModule.GetMap(debugThreadsIDsMapName)
		//if err != nil {
		//	return fmt.Errorf("get debug pids map: %w", err)
		//}
		//if err := debugThreadsIDs.SetMaxEntries(maxProcesses); err != nil {
		//	return fmt.Errorf("resize debug threads ids map from default to %d elements: %w", maxProcesses, err)
		//}
	}
	return nil
}

func (m *Maps) Create() error {
	m.debugPIDs = m.modules.ParcaNativeObjects.ParcaNativeMaps.DebugThreadsIds
	m.StackCounts = m.modules.ParcaNativeObjects.ParcaNativeMaps.StackCounts
	m.stackTraces = m.modules.ParcaNativeObjects.ParcaNativeMaps.StackTraces
	m.eventsCount = m.modules.ParcaNativeObjects.ParcaNativeMaps.EventsCount
	m.unwindShards = m.modules.ParcaNativeObjects.ParcaNativeMaps.UnwindInfoChunks
	m.unwindTables = m.modules.ParcaNativeObjects.ParcaNativeMaps.UnwindTables
	m.processInfo = m.modules.ParcaNativeObjects.ParcaNativeMaps.ProcessInfo

	return nil
}

// AddInterpreter adds the interpreter information to the relevant BPF maps.
// It is a lookup table for the BPF program to find the interpreter information
// for corresponding PID.
// Process information is stored in a separate map and needs to be updated
// separately.
func (m *Maps) AddInterpreter(pid int, interpreter runtime.Interpreter) error {
	if v, ok := m.syncedInterpreters.Get(pid); ok && v == interpreter {
		return nil
	}

	i, err := m.indexForInterpreter(interpreter)
	if err != nil {
		return fmt.Errorf("index for interpreter version: %w", err)
	}

	switch interpreter.Type {
	case runtime.InterpreterRuby:
		procData := rbperf.ProcessData{
			RbFrameAddr: interpreter.MainThreadAddress,
			RbVersion:   i,
			Padding_:    [4]byte{0, 0, 0, 0},
			StartTime:   0, // Unused as of now.
		}
		level.Debug(m.logger).Log("msg", "Ruby Version Offset", "pid", pid, "version_offset_index", i)
		if err := m.setRbperfProcessData(pid, procData); err != nil {
			return err
		}
		m.syncedInterpreters.Add(pid, interpreter)
	case runtime.InterpreterPython:
		interpreterInfo := pyperf.InterpreterInfo{
			ThreadStateAddr:      interpreter.MainThreadAddress,
			PyVersionOffsetIndex: i,
		}
		level.Debug(m.logger).Log("msg", "Python Version Offset", "pid", pid, "version_offset_index", i)
		if err := m.setPyperfIntepreterInfo(pid, interpreterInfo); err != nil {
			return err
		}
		m.syncedInterpreters.Add(pid, interpreter)
	default:
		return fmt.Errorf("invalid interpreter name: %d", interpreter.Type)
	}
	return nil
}

func (m *Maps) indexForInterpreter(interpreter runtime.Interpreter) (uint32, error) {
	var mapping map[string]uint32

	version, err := semver.NewVersion(interpreter.Version)
	if err != nil {
		return 0, fmt.Errorf("parse version: %w", err)
	}
	switch interpreter.Type {
	case runtime.InterpreterRuby:
		mapping = m.rubyVersionToOffsetIndex
		if i, ok := mapping[fmt.Sprintf("%d.%d.%d", version.Major(), version.Minor(), version.Patch())]; ok {
			return i, nil
		}
	case runtime.InterpreterPython:
		mapping = m.pythonVersionToOffsetIndex
		if i, ok := mapping[fmt.Sprintf("%d.%d", version.Major(), version.Minor())]; ok {
			return i, nil
		}
	default:
		return 0, fmt.Errorf("invalid interpreter name: %d", interpreter.Type)
	}

	return 0, fmt.Errorf("unknown version %s", version.String())
}

func (m *Maps) SetDebugPIDs(pids []int) error {
	return nil
}

// ReadStack reads the walked stacktrace into the given buffer.
func (m *Maps) ReadStack(stackID uint64, stack []uint64) error {
	if stackID == 0 {
		return ErrUnwindFailed
	}

	stackBytes, err := m.stackTraces.LookupBytes(stackID)
	if err != nil {
		return fmt.Errorf("read user stack trace, %w: %w", err, ErrMissing)
	}

	var rawStackWithLenth stackTraceWithLength
	if err := binary.Read(bytes.NewBuffer(stackBytes), m.byteOrder, &rawStackWithLenth); err != nil {
		return fmt.Errorf("read user stack bytes, %w: %w", err, ErrUnrecoverable)
	}

	for i, addr := range rawStackWithLenth.Addrs {
		if i >= bpfprograms.StackDepth || i >= int(rawStackWithLenth.Len) || addr == 0 {
			break
		}
		stack[i] = addr
	}

	return nil
}

// cStringToGo converts a C string in a buffer to a Go string,
// making sure we do not read past NUL, as this is a statically
// sized buffer that might not be full.
func cStringToGo(in []uint8) string {
	var buffer bytes.Buffer
	for _, datum := range in {
		if datum == 0 {
			break
		}
		buffer.WriteByte(datum)
	}
	return buffer.String()
}

// InterpreterSymbolTable retrieves the whole symbol table in full so we
// can construct a fast frameId -> Frame lookup table.

// PERF: This code presents (at least) presents two possible performance
// opportunities that we should measure.
//
// - Preallocating the lookup table.
// - Batch the BPF map calls to read and update them.
func (m *Maps) InterpreterSymbolTable() (profile.InterpreterSymbolTable, error) {
	interpreterFrames := make(profile.InterpreterSymbolTable)

	symbolTable := m.modules.ParcaNativeObjects.ParcaNativeMaps.SymbolTable
	it := symbolTable.Iterate()
	keyBytes := make([]byte, symbolTable.KeySize())
	valueBytes := make([]byte, symbolTable.ValueSize())
	for it.Next(keyBytes, valueBytes) {
		symbol := bpf.Symbol{}
		if err := binary.Read(bytes.NewBuffer(keyBytes), m.byteOrder, &symbol); err != nil {
			return interpreterFrames, fmt.Errorf("read interpreter stack bytes, %w: %w", err, ErrUnrecoverable)
		}

		symbolIndex := uint32(0)
		if err := binary.Read(bytes.NewBuffer(valueBytes), m.byteOrder, &symbolIndex); err != nil {
			return interpreterFrames, fmt.Errorf("read interpreter frame bytes, %w: %w", err, ErrUnrecoverable)
		}
		interpreterFrames[symbolIndex] = &profile.Function{
			ModuleName: cStringToGo(symbol.ClassName[:]),
			Name:       cStringToGo(symbol.MethodName[:]),
			Filename:   cStringToGo(symbol.Path[:]),
		}
	}

	return interpreterFrames, nil
}

// ReadStackCount reads the value of the given key from the counts ebpf map.
func (m *Maps) ReadStackCount(keyBytes []byte) (uint64, error) {
	valueBytes, err := m.StackCounts.LookupBytes(keyBytes)
	if err != nil {
		return 0, fmt.Errorf("get count value: %w", err)
	}
	return m.byteOrder.Uint64(valueBytes), nil
}

func (m *Maps) FinalizeProfileLoop() error {
	m.profilingRoundsWithoutUnwindInfoReset++
	m.profilingRoundsWithoutProcessInfoReset++

	var result error

	if err := m.cleanStacks(); err != nil {
		result = errors.Join(result, err)
	}

	if err := m.cleanEventsCount(); err != nil {
		result = errors.Join(result, err)
	}

	return result
}

func (m *Maps) cleanStacks() error {
	var result error

	if err := clearMap(m.stackTraces); err != nil {
		m.metrics.mapCleanErrors.WithLabelValues("stackTraces").Inc()
		result = errors.Join(result, err)
	}

	if err := clearMap(m.StackCounts); err != nil {
		m.metrics.mapCleanErrors.WithLabelValues("StackCounts").Inc()
		result = errors.Join(result, err)
	}

	return result
}

func clearMap(bpfMap *ebpf.Map) error {
	// BPF iterators need the previous value to iterate to the next, so we
	// can only delete the "previous" item once we've already iterated to
	// the next.

	it := bpfMap.Iterate()
	var keys [][]byte
	keyBytes := make([]byte, bpfMap.KeySize())
	valueBytes := make([]byte, bpfMap.ValueSize())
	for it.Next(keyBytes, valueBytes) {
		itc := make([]byte, len(keyBytes))
		copy(itc, keyBytes)
		keys = append(keys, itc)
	}

	for _, key := range keys {
		err := bpfMap.Delete(key)
		if err != nil && !errors.Is(err, syscall.ENOENT) {
			fmt.Printf("%s\n", fmt.Errorf("failed to delete map key: %w", err))
		}
	}
	return nil
}

func (m *Maps) cleanEventsCount() error {
	if err := clearMap(m.eventsCount); err != nil {
		m.metrics.mapCleanErrors.WithLabelValues("eventsCount").Inc()
		return err
	}
	return nil
}

func (m *Maps) cleanProcessInfo() error {
	if err := clearMap(m.processInfo); err != nil {
		m.metrics.mapCleanErrors.WithLabelValues("processInfo").Inc()
		return err
	}
	return nil
}

func (m *Maps) cleanShardInfo() error {
	// unwindShards
	if err := clearMap(m.unwindShards); err != nil {
		m.metrics.mapCleanErrors.WithLabelValues("unwindShards").Inc()
		return err
	}
	return nil
}

func (m *Maps) resetMappingInfoBuffer() error {
	// Extend length to match the capacity.
	m.mappingInfoMemory = m.mappingInfoMemory[:cap(m.mappingInfoMemory)]

	// Zero it.
	for i := 0; i < cap(m.mappingInfoMemory); i++ {
		m.mappingInfoMemory[i] = 0
	}

	// Reset length.
	m.mappingInfoMemory = m.mappingInfoMemory[:0]
	return nil
}

// RefreshProcessInfo updates the process information such as mappings and unwind
// information if the executable mappings have changed.
func (m *Maps) RefreshProcessInfo(pid int, shouldUseFPByDefault bool) {
	level.Debug(m.logger).Log("msg", "refreshing process info", "pid", pid)

	cachedHash, _ := m.processCache.Get(pid)

	proc, err := procfs.NewProc(pid)
	if err != nil {
		return
	}
	mappings, err := proc.ProcMaps()
	if err != nil {
		return
	}
	executableMappings := unwind.ListExecutableMappings(mappings)
	currentHash, err := executableMappings.Hash()
	if err != nil {
		m.metrics.refreshProcessInfoErrors.WithLabelValues(labelHash).Inc()
		level.Error(m.logger).Log("msg", "executableMappings hash failed", "err", err)
		return
	}

	if cachedHash != currentHash {
		err := m.AddUnwindTableForProcess(pid, executableMappings, false, shouldUseFPByDefault)
		if err != nil {
			m.metrics.refreshProcessInfoErrors.WithLabelValues(labelUnwindTableAdd).Inc()
			level.Error(m.logger).Log("msg", "addUnwindTableForProcess failed", "err", err)
		}
	}
}

// 1. Find executable sections
// 2. For each section, generate compact table
// 3. Add table to maps
// 4. Add map metadata to process
func (m *Maps) AddUnwindTableForProcess(pid int, executableMappings unwind.ExecutableMappings, checkCache, shouldUseFPByDefault bool) error {
	// Notes:
	//	- perhaps we could cache based on `start_at` (but parsing this procfs file properly
	// is challenging if the process name contains spaces, etc).
	//  - PIDs can be recycled.

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if checkCache {
		if _, exists := m.processCache.Get(pid); exists {
			level.Debug(m.logger).Log("msg", "process already cached", "pid", pid)
			return nil
		}
	}

	if executableMappings == nil {
		proc, err := procfs.NewProc(pid)
		if err != nil {
			return err
		}
		mappings, err := proc.ProcMaps()
		if err != nil {
			return err
		}
		executableMappings = unwind.ListExecutableMappings(mappings)
	}

	// Clean up the mapping information.
	if err := m.resetMappingInfoBuffer(); err != nil {
		level.Error(m.logger).Log("msg", "resetMappingInfoBuffer failed", "err", err)
	}

	// Important: the below *must* be called before setUnwindTable.
	var isJITCompiler uint64
	if executableMappings.HasJITted() {
		isJITCompiler = 1
	}

	if len(executableMappings) >= maxMappingsPerProcess {
		return ErrTooManyExecutableMappings
	}

	mappingInfoMemory := m.mappingInfoMemory.Slice(mappingInfoSizeBytes)

	var lol uint64
	if shouldUseFPByDefault {
		lol = 1
	}

	// .should_use_fp_by_default
	mappingInfoMemory.PutUint64(lol)

	// .is_jit_compiler
	mappingInfoMemory.PutUint64(isJITCompiler)
	// .interpreter_type
	var interpreterType uint64
	// Important: the below *must* be called after AddInterpreter.
	interp, ok := m.syncedInterpreters.Get(pid)
	if ok {
		interpreterType = uint64(interp.Type)
	}
	mappingInfoMemory.PutUint64(interpreterType)
	// .len
	mappingInfoMemory.PutUint64(uint64(len(executableMappings)))

	for _, executableMapping := range executableMappings {
		if executableMapping.IsJITDump() {
			continue
		}
		if err := m.setUnwindTableForMapping(&mappingInfoMemory, pid, executableMapping); err != nil {
			return fmt.Errorf("setUnwindTableForMapping for executable %s starting at 0x%x failed: %w", executableMapping.Executable, executableMapping.StartAddr, err)
		}
	}

	// TODO(javierhonduco): There's a small window where it's possible that
	// the unwind information hasn't been written to the map while the process
	// information has. During this window unwinding might fail. Particularly,
	// this is a problem when we decide to delay regenerating the DWARF state
	// when running out of shards.
	if err := m.processInfo.Update(uint32(pid), m.mappingInfoMemory, ebpf.UpdateAny); err != nil {
		if errors.Is(err, syscall.E2BIG) {
			if m.profilingRoundsWithoutProcessInfoReset < minRoundsBeforeRedoingProcessInformation {
				level.Debug(m.logger).Log("msg", "not enough profile loops, we need to wait to reset proc info")
				m.waitingToResetProcessInfo = true
				return nil
			}

			if m.waitingToResetProcessInfo {
				level.Debug(m.logger).Log("msg", "no need to wait anymore to reset proc info")
				m.waitingToResetProcessInfo = false
				m.profilingRoundsWithoutProcessInfoReset = 0
			}

			m.processCache.Purge()
			cleanErr := m.cleanProcessInfo()
			level.Debug(m.logger).Log("msg", "resetting process information", "cleanErr", cleanErr)

			// Next call will populate the process info.
			return nil
		}
		return fmt.Errorf("update processInfo: %w", err)
	}

	mapsHash, err := executableMappings.Hash()
	if err != nil {
		return fmt.Errorf("maps hash: %w", err)
	}
	m.processCache.Add(pid, mapsHash)
	return nil
}

// writeUnwindTableRow writes a compact unwind table row to the provided slice.
//
// Note: we are avoiding `binary.Write` and prefer to use the lower level APIs
// to avoid allocations and CPU spent in the reflection code paths as well as
// in the allocations for the intermediate buffers.
func (m *Maps) writeUnwindTableRow(rowSlice *profiler.EfficientBuffer, row unwind.CompactUnwindTableRow, arch elf.Machine) {
	// .pc
	rowSlice.PutUint64(row.Pc())
	if arch == elf.EM_AARCH64 {
		// .lr_offset
		rowSlice.PutInt16(row.LrOffset())
	}
	// .cfa_type
	rowSlice.PutUint8(row.CfaType())
	// .rbp_type
	rowSlice.PutUint8(row.RbpType())
	// .cfa_offset
	rowSlice.PutInt16(row.CfaOffset())
	// .rbp_offset
	rowSlice.PutInt16(row.RbpOffset())
}

// writeMapping writes the memory mapping information to the provided buffer.
//
// Note: we write field by field to avoid the expensive reflection code paths
// when writing structs using `binary.Write`.
func (m *Maps) writeMapping(buf *profiler.EfficientBuffer, loadAddress, startAddr, endAddr, executableID, type_ uint64) {
	// .load_address
	buf.PutUint64(loadAddress)
	// .begin
	buf.PutUint64(startAddr)
	// .end
	buf.PutUint64(endAddr)
	// .executable_id
	buf.PutUint64(executableID)
	// .type
	buf.PutUint64(type_)
}

// mappingID returns the internal identifier for a memory mapping.
//
// It will either return the already produced ID or generate a new
// one while indicating whether it was already seen or not.
//
// This allows us to reuse the unwind tables for the mappings we
// see.
func (m *Maps) mappingID(buildID string) (uint64, bool) {
	_, alreadySeenMapping := m.buildIDMapping[buildID]
	if alreadySeenMapping {
		level.Debug(m.logger).Log("msg", "mapping caching, seen before", "buildID", buildID)
		m.referencedMappings += 1
	} else {
		level.Debug(m.logger).Log("msg", "mapping caching, new", "buildID", buildID)
		m.buildIDMapping[buildID] = m.executableID
	}

	return m.buildIDMapping[buildID], alreadySeenMapping
}

// resetInFlightBuffer zeroes and resets the length of the
// in-flight shard.
func (m *Maps) resetInFlightBuffer() error {
	// Extend length to match the capacity.
	m.unwindInfoMemory = m.unwindInfoMemory[:cap(m.unwindInfoMemory)]

	// Zero it.
	for i := 0; i < cap(m.unwindInfoMemory); i++ {
		m.unwindInfoMemory[i] = 0
	}

	// Reset slice's len.
	m.unwindInfoMemory = m.unwindInfoMemory[:0]
	return nil
}

// PersistUnwindTable calls persistUnwindTable but holding the mutex
// to ensure that shared state is mutated safely.
//
// Never use this function from addUnwindTableForProcess, as it holds
// this same mutex.
func (m *Maps) PersistUnwindTable() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return m.persistUnwindTable()
}

// persistUnwindTable writes the current in-flight, writable shard
// to the corresponding BPF map's shard.
//
// Note: as of now, this must be called in two situations:
//   - In the callsite, once we are done with generating the unwind
//     tables (see PersistUnwindTable).
//   - Whenever the current in-flight shard is full, before we wipe
//     it and start reusing it.
func (m *Maps) persistUnwindTable() error {
	totalRows := len(m.unwindInfoMemory) / m.compactUnwindRowSizeBytes
	if totalRows > maxUnwindTableSize {
		panic("totalRows > maxUnwindTableSize should never happen")
	}
	level.Debug(m.logger).Log("msg", "PersistUnwindTable called", "live unwind rows", totalRows)

	if totalRows == 0 {
		return nil
	}

	shardIndex := m.shardIndex

	err := m.unwindTables.Update(shardIndex, m.unwindInfoMemory[:cap(m.unwindInfoMemory)], ebpf.UpdateAny)
	if err != nil {
		if errors.Is(err, syscall.E2BIG) {
			// If we need to wipe all state because we run out of shards, let's only do it after few
			// profiling rounds.
			//
			// It's the responsibility of the caller to ensure that the processes to be profiled have
			// a fair ordering.
			if m.profilingRoundsWithoutUnwindInfoReset < minRoundsBeforeRedoingUnwindInfo {
				level.Debug(m.logger).Log("msg", "not enough profile loops, we need to wait to reset unwind info")
				m.waitingToResetUnwindInfo = true
				return ErrNeedMoreProfilingRounds
			}

			if m.waitingToResetUnwindInfo {
				level.Debug(m.logger).Log("msg", "no need to wait anymore to reset unwind info")
				m.waitingToResetUnwindInfo = false
				m.profilingRoundsWithoutUnwindInfoReset = 0
			}

			if err := m.resetUnwindState(); err != nil {
				level.Error(m.logger).Log("msg", "resetUnwindState failed", "err", err)
				return err
			}
			return nil
		}
		return fmt.Errorf("update unwind tables: %w", err)
	}

	return nil
}

func (m *Maps) resetUnwindState() error {
	m.processCache.Purge()
	m.buildIDMapping = make(map[string]uint64)
	m.shardIndex = 0
	m.executableID = 0
	if err := m.resetInFlightBuffer(); err != nil {
		level.Error(m.logger).Log("msg", "resetInFlightBuffer failed", "err", err)
	}

	m.lowIndex = 0
	m.highIndex = 0
	// Other stats
	m.totalEntries = 0
	m.uniqueMappings = 0
	m.referencedMappings = 0

	if err := m.cleanProcessInfo(); err != nil {
		level.Error(m.logger).Log("msg", "cleanProcessInfo failed", "err", err)
		return err
	}
	if err := m.cleanShardInfo(); err != nil {
		level.Error(m.logger).Log("msg", "cleanShardInfo failed", "err", err)
		return err
	}
	if err := m.cleanStacks(); err != nil {
		level.Error(m.logger).Log("msg", "cleanStacks failed", "err", err)
		return err
	}

	return nil
}

// availableEntries returns how many entries we have left
// in the in-flight shard.
func (m *Maps) availableEntries() uint64 {
	return maxUnwindTableSize - m.highIndex
}

// assertInvariants checks that some invariants that should
// always be true during the execution of the program are held.
func (m *Maps) assertInvariants() {
	if m.highIndex > maxUnwindTableSize {
		panic(fmt.Sprintf("m.highIndex (%d)> 250k, this should never happen", m.highIndex))
	}
	tableSize := len(m.unwindInfoMemory) / m.compactUnwindRowSizeBytes
	if tableSize > maxUnwindTableSize {
		panic(fmt.Sprintf("unwindInfoBuf has %d entries, more than the 250k max", tableSize))
	}
	if m.availableEntries() == 0 {
		panic("no space left in the in-flight shard, this should never happen")
	}
}

// allocateNewShard uses a new shard. This must be called whenever we ran out of space
// in the current "live" shard, or when we want to avoid splitting a function's unwind
// information.
func (m *Maps) allocateNewShard() error {
	err := m.persistUnwindTable()
	if err != nil {
		return fmt.Errorf("failed to write unwind table: %w", err)
	}

	if err := m.resetInFlightBuffer(); err != nil {
		level.Error(m.logger).Log("msg", "resetInFlightBuffer failed", "err", err)
	}

	m.shardIndex++
	m.lowIndex = 0
	m.highIndex = 0

	if m.shardIndex == m.maxUnwindShards {
		level.Debug(m.logger).Log("msg", "next shard persist will reset the unwind info")
	}

	return nil
}

// setUnwindTableForMapping sets all the necessary metadata and unwind tables, if needed
// to make DWARF unwinding work, such as:
//
//   - Continue appending information to the executable mapping information for a process.
//   - Add mapping information.
//   - If unwind table is already present, we are done here, otherwise, we generate the
//     unwind table for this executable and write to the in-flight shard.
//
// Notes:
//
// - This function is *not* safe to be called concurrently, the caller, addUnwindTableForProcess
// uses a mutex to ensure safe data access.
func (m *Maps) setUnwindTableForMapping(buf *profiler.EfficientBuffer, pid int, mapping *unwind.ExecutableMapping) error {
	level.Debug(m.logger).Log("msg", "setUnwindTable called", "shards", m.shardIndex, "max shards", m.maxUnwindShards, "sum of unwind rows", m.totalEntries)

	// Deal with mappings that are not filed backed. They don't have unwind
	// information.
	if mapping.IsNotFileBacked() {
		var type_ uint64
		if mapping.IsJITted() {
			level.Debug(m.logger).Log("msg", "jit section", "pid", pid)
			type_ = mappingTypeJITted
		}
		if mapping.IsSpecial() {
			level.Debug(m.logger).Log("msg", "special section", "pid", pid)
			type_ = mappingTypeSpecial
		}

		m.writeMapping(buf, mapping.LoadAddr, mapping.StartAddr, mapping.EndAddr, uint64(0), type_)
		return nil
	}

	// Deal with mappings that are backed by a file and might contain unwind
	// information.
	fullExecutablePath := path.Join("/proc/", strconv.Itoa(pid), "/root/", mapping.Executable)

	f, err := os.Open(fullExecutablePath)
	if err != nil {
		return err
	}

	ef, err := elf.NewFile(f)
	var elfErr *elf.FormatError
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if errors.As(err, &elfErr) {
			level.Debug(m.logger).Log("msg", "bad ELF file format", "err", err)
			return nil
		}
		return fmt.Errorf("elf.Open failed: %w", err)
	}
	buildID, err := buildid.FromELF(ef)
	if err != nil {
		return fmt.Errorf("BuildID failed %s: %w", fullExecutablePath, err)
	}

	// Find the adjusted load address.
	aslrElegible := elfreader.IsASLRElegibleElf(ef)

	adjustedLoadAddress := uint64(0)
	if mapping.IsMainObject() {
		level.Debug(m.logger).Log("msg", "dealing with main object", "mapping", mapping)

		if aslrElegible {
			adjustedLoadAddress = mapping.LoadAddr
		}
	} else {
		adjustedLoadAddress = mapping.LoadAddr
	}

	level.Debug(m.logger).Log("msg", "adding memory mappings in for executable", "executableID", m.executableID, "buildID", buildID, "executable", mapping.Executable)

	// Add the memory mapping information.
	foundexecutableID, mappingAlreadySeen := m.mappingID(buildID)

	m.writeMapping(buf, adjustedLoadAddress, mapping.StartAddr, mapping.EndAddr, foundexecutableID, uint64(0))

	// Generated and add the unwind table, if needed.
	if !mappingAlreadySeen {
		unwindShardsValBuf := new(bytes.Buffer)
		unwindShardsValBuf.Grow(unwindShardsSizeBytes)

		// Generate the unwind table.
		// PERF(javierhonduco): Not reusing a buffer here yet, let's profile and decide whether this
		// change would be worth it.
		ut, arch, err := unwind.GenerateCompactUnwindTable(fullExecutablePath)
		level.Debug(m.logger).Log("msg", "found unwind entries", "executable", mapping.Executable, "len", len(ut))

		if err != nil {
			if !errors.Is(err, unwind.ErrNoFDEsFound) {
				return nil
			}
			if errors.Is(err, unwind.ErrEhFrameSectionNotFound) {
				return nil
			}
			return nil
		}

		if len(ut) == 0 {
			return nil
		}

		chunkIndex := 0

		var (
			currentChunk unwind.CompactUnwindTable
			restChunks   unwind.CompactUnwindTable
		)

		restChunks = ut

		for {
			if m.waitingToResetUnwindInfo {
				return ErrNeedMoreProfilingRounds
			}
			maxThreshold := min(len(restChunks), int(m.availableEntries()))

			if maxThreshold == 0 {
				level.Debug(m.logger).Log("msg", "done with the last chunk")
				break
			}

			// Find the end of the last function and split the unwind table
			// at that index.
			currentChunkCandidate := restChunks[:maxThreshold]
			threshold := maxThreshold
			for i := maxThreshold - 1; i >= 0; i-- {
				if currentChunkCandidate[i].IsEndOfFDEMarker() {
					break
				}
				threshold--
			}

			// We couldn't find a full function in the current unwind information.
			// As we can't split an unwind table mid-function, let's create a new
			// shard.
			if threshold == 0 {
				level.Debug(m.logger).Log("msg", "creating a new shard to avoid splitting the unwind table for a function")
				if err := m.allocateNewShard(); err != nil {
					return err
				}
				continue
			}

			currentChunk = restChunks[:threshold]
			restChunks = restChunks[threshold:]

			if currentChunk[0].IsEndOfFDEMarker() {
				level.Error(m.logger).Log("msg", "first row of a chunk should not be a marker")
			}

			if !currentChunk[len(currentChunk)-1].IsEndOfFDEMarker() {
				level.Error(m.logger).Log("msg", "last row of a chunk should always be a marker")
			}

			m.assertInvariants()

			if chunkIndex >= maxUnwindTableChunks {
				level.Error(m.logger).Log("msg", "have more chunks than the max", "chunks", chunkIndex, "maxChunks", maxUnwindTableChunks)
				// TODO(javierhonduco): not returning an error right now, but let's handle this later on.
			}

			level.Debug(m.logger).Log("current chunk size", len(currentChunk))
			level.Debug(m.logger).Log("rest of chunk size", len(restChunks))

			m.totalEntries += uint64(len(currentChunk))

			m.highIndex += uint64(len(currentChunk))
			level.Debug(m.logger).Log("lowindex", m.lowIndex)
			level.Debug(m.logger).Log("highIndex", m.highIndex)

			// Add shard information.

			level.Debug(m.logger).Log("executableID", m.executableID, "executable", mapping.Executable, "current shard", chunkIndex)

			// Dealing with the first chunk, we must add the lowest known PC.
			minPc := currentChunk[0].Pc()
			if minPc == 0 {
				panic("maxPC can't be zero")
			}
			// .low_pc
			if err := binary.Write(unwindShardsValBuf, m.byteOrder, minPc); err != nil {
				return fmt.Errorf("write shards .low_pc bytes: %w", err)
			}

			// Dealing with the last chunk, we must add the highest known PC.
			maxPc := currentChunk[len(currentChunk)-1].Pc()
			// .high_pc
			if err := binary.Write(unwindShardsValBuf, m.byteOrder, maxPc); err != nil {
				return fmt.Errorf("write shards .high_pc bytes: %w", err)
			}

			// .shard_index
			if err := binary.Write(unwindShardsValBuf, m.byteOrder, m.shardIndex); err != nil {
				return fmt.Errorf("write shards .shard_index bytes: %w", err)
			}

			// .low_index
			if err := binary.Write(unwindShardsValBuf, m.byteOrder, m.lowIndex); err != nil {
				return fmt.Errorf("write shards .low_index bytes: %w", err)
			}
			// .high_index
			if err := binary.Write(unwindShardsValBuf, m.byteOrder, m.highIndex); err != nil {
				return fmt.Errorf("write shards .high_index bytes: %w", err)
			}

			m.lowIndex = m.highIndex

			// Write unwind table.
			for _, row := range currentChunk {
				// Get a slice of the bytes we need for this row.
				rowSlice := m.unwindInfoMemory.Slice(m.compactUnwindRowSizeBytes)
				m.writeUnwindTableRow(&rowSlice, row, arch)
			}

			// We ran out of space in the current shard. Let's allocate a new one.
			if m.availableEntries() == 0 {
				level.Debug(m.logger).Log("msg", "creating a new shard as we ran out of space")

				if err := m.allocateNewShard(); err != nil {
					return err
				}
			}

			chunkIndex++
		}

		executableID := m.executableID
		if err := m.unwindShards.Update(
			executableID,
			unwindShardsValBuf.Bytes()[:unwindShardsSizeBytes], ebpf.UpdateAny); err != nil {
			return fmt.Errorf("failed to update unwind shard: %w", err)
		}

		m.executableID++
		m.uniqueMappings++
	}

	return nil
}
