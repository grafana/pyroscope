package ebpfspy

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/perf"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/phlare/ebpf/sd"
	"github.com/grafana/phlare/ebpf/symtab"
	"github.com/samber/lo"
)

type pyPerf struct {
	rd             *perf.Reader
	logger         log.Logger
	pidDataHashMap *ebpf.Map
	symbolsHashMp  *ebpf.Map

	events     []*ProfilePyEvent
	eventsLock sync.Mutex
}

func newPyPerf(logger log.Logger, perfEventMap *ebpf.Map, pidDataHasMap *ebpf.Map, symbolsHashMap *ebpf.Map) (*pyPerf, error) {
	rd, err := perf.NewReader(perfEventMap, 4*os.Getpagesize())
	if err != nil {
		return nil, fmt.Errorf("perf new reader: %w", err)
	}
	res := &pyPerf{
		rd:             rd,
		logger:         logger,
		pidDataHashMap: pidDataHasMap,
		symbolsHashMp:  symbolsHashMap,
	}
	go func() {
		res.loop()
	}()
	return res, nil
}

func (s *pyPerf) setPythonPIDs(pids []int) {
	for _, pid := range pids {
		err := s.addPythonPID(pid)
		if err != nil {
			_ = level.Error(s.logger).Log("msg", "[pyperf] adding python pid", "pid", pid, "err", err)
		}
	}
}

func GetPyPerfPidData(pid int) (*ProfilePyPidData, error) {
	mapsPath := fmt.Sprintf("/proc/%d/maps", pid)
	maps, err := os.ReadFile(mapsPath) //todo should we streaming parse it?
	if err != nil {
		return nil, fmt.Errorf("reading proc maps %s: %w", mapsPath, err)
	}
	modules, err := symtab.ParseProcMapsExecutableModules(maps, false)
	if err != nil {
		return nil, fmt.Errorf("parsing proc maps %s: %w", mapsPath, err)
	}

	var found *symtab.ProcMap
	for _, module := range modules {
		if len(module.Pathname) > 0 && module.Pathname[0] == '/' && strings.Index(module.Pathname, "libpython3.6") != -1 {
			found = module
			break
		}
	}
	if found == nil { // todo other versions
		return nil, fmt.Errorf("libpython3.6 not found")
	}
	path := fmt.Sprintf("/proc/%d/root%s", pid, found.Pathname)
	ef, err := elf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening elf %s: %w", path, err)
	}
	symbols, err := ef.Symbols() // todo reuse parser from symtab, make it optionally streaming
	if err != nil {
		return nil, fmt.Errorf("reading symbols from elf %s: %w", path, err)
	}

	data := &ProfilePyPidData{}
	for _, symbol := range symbols {
		switch symbol.Name {
		case "autoTLSkey":
			data.TlsKeyAddr = found.StartAddr + symbol.Value
		case "_PyThreadState_Current":
			data.CurrentStateAddr = found.StartAddr + symbol.Value
		case "gil_locked":
			data.GilLockedAddr = found.StartAddr + symbol.Value
		case "gil_last_holder":
			data.GilLastHolderAddr = found.StartAddr + symbol.Value
		default:
			continue
		}
	}
	if data.TlsKeyAddr == 0 || data.CurrentStateAddr == 0 || data.GilLockedAddr == 0 || data.GilLastHolderAddr == 0 {
		return nil, fmt.Errorf("missing symbols")
	}
	offsetConfig := py36OffsetConfig
	data.Offsets.PyObjectType = offsetConfig.PyObject_type
	data.Offsets.PyTypeObjectName = offsetConfig.PyTypeObject_name
	data.Offsets.PyThreadStateFrame = offsetConfig.PyThreadState_frame
	data.Offsets.PyThreadStateThread = offsetConfig.PyThreadState_thread
	data.Offsets.PyFrameObjectBack = offsetConfig.PyFrameObject_back
	data.Offsets.PyFrameObjectCode = offsetConfig.PyFrameObject_code
	data.Offsets.PyFrameObjectLineno = offsetConfig.PyFrameObject_lineno
	data.Offsets.PyFrameObjectLocalsplus = offsetConfig.PyFrameObject_localsplus
	data.Offsets.PyCodeObjectFilename = offsetConfig.PyCodeObject_filename
	data.Offsets.PyCodeObjectName = offsetConfig.PyCodeObject_name
	data.Offsets.PyCodeObjectVarnames = offsetConfig.PyCodeObject_varnames
	data.Offsets.PyTupleObjectItem = offsetConfig.PyTupleObject_item
	data.Offsets.StringData = offsetConfig.String_data
	data.Offsets.StringSize = offsetConfig.String_size
	return data, nil
}

func (s *pyPerf) addPythonPID(pid int) error {
	//todo do not do this multiple times
	data, err := GetPyPerfPidData(pid)
	if err != nil {
		return fmt.Errorf("error collecting python data %w", err)
	}
	err = s.pidDataHashMap.Update(uint32(pid), data, ebpf.UpdateAny)
	if err != nil {
		return fmt.Errorf("updating pid data hash map: %w", err)
	}
	return nil
}

func (s *pyPerf) loop() {
	defer s.rd.Close()

	for {
		record, err := s.rd.Read()
		if err != nil {
			if errors.Is(err, perf.ErrClosed) {
				return
			}
			_ = level.Error(s.logger).Log("msg", "[pyperf] reading from perf event reader", "err", err)
			continue
		}

		if record.LostSamples != 0 {
			_ = level.Debug(s.logger).Log("msg", "[pyperf] perf event ring buffer full, dropped samples", "n", record.LostSamples)
			continue
		}

		// Parse the perf event entry into a bpfEvent structure.
		event := &ProfilePyEvent{}
		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, event); err != nil {
			_ = level.Error(s.logger).Log("msg", "[pyperf] parsing perf event record", "err", err)
			continue
		}
		s.eventsLock.Lock()
		s.events = append(s.events, event)
		s.eventsLock.Unlock()
	}

}

func (s *pyPerf) Close() {
	_ = s.rd.Close()
}

func (s *pyPerf) getSymbols() map[uint32]string {
	var (
		m       = s.symbolsHashMp
		mapSize = m.MaxEntries()
		nextKey = ProfilePySymbol{}
	)
	keys := make([]ProfilePySymbol, mapSize)
	values := make([]uint32, mapSize)

	opts := &ebpf.BatchOptions{}
	n, _ := m.BatchLookup(nil, &nextKey, keys, values, opts)
	if n > 0 {
		level.Debug(s.logger).Log(
			"msg", "getCountsMapValues BatchLookupAndDelete",
			"count", n,
		)
		res := make(map[uint32]string, n)
		for i := 0; i < int(n); i++ {
			k := values[i]
			//name := strFromInt8(keys[i].Name[:])
			//fmt.Printf("sym %d %s\n", k, name)
			//file := strFromInt8(keys[i].File[:])
			//className := strFromInt8(keys[i].Classname[:])
			//_ = className
			res[k] = fmt.Sprintf("%s!%s", keys[i].Name, keys[i].File) // todo propper format
		}
		return res
	}
	// batch not supported
	//todo implement
	panic("implement me")
	return nil
}

func strFromInt8(file []int8) any {
	u8 := make([]uint8, 0, len(file))
	for _, v := range file {
		if v == 0 {
			break
		}
		u8 = append(u8, uint8(v))
	}
	return string(u8)
}

func (s *pyPerf) CollectProfiles(cb func(t *sd.Target, stack []string, value uint64, pid uint32), targetFinder sd.TargetFinder) {
	events := s.resetEvents()
	symbols := s.getSymbols()
	sb := stackBuilder{}
	for _, event := range events {
		target := targetFinder.FindTarget(event.Pid)
		if target == nil {
			continue
		}

		sb.rest()
		//sb.append(getComm(&event.Comm))
		// todo get comm from pid
		for _, symID := range event.Stack {
			if symID == 0 {
				break
			}
			sym := symbols[symID]
			if sym == "" {
				sb.append(fmt.Sprintf("[%d] unknown ", symID))
			} else {
				sb.append(sym)
			}
		}

		if len(sb.stack) == 1 { //todo backport drop option?
			continue // only comm
		}
		lo.Reverse(sb.stack)
		cb(target, sb.stack, uint64(1), event.Pid)
	}
}

func (s *pyPerf) resetEvents() []*ProfilePyEvent {
	s.eventsLock.Lock()
	eventsCopy := make([]*ProfilePyEvent, len(s.events))
	copy(eventsCopy, s.events)
	for i := range s.events {
		s.events[i] = nil
	}
	s.events = s.events[:0]
	s.eventsLock.Unlock()
	return eventsCopy
}

//extern const OffsetConfig kPy36OffsetConfig = {
//.PyObject_type = 8,               // offsetof(PyObject, ob_type)
//.PyTypeObject_name = 24,          // offsetof(PyTypeObject, tp_name)
//.PyThreadState_frame = 24,        // offsetof(PyThreadState, frame)
//.PyThreadState_thread = 152,      // offsetof(PyThreadState, thread_id)
//.PyFrameObject_back = 24,         // offsetof(PyFrameObject, f_back)
//.PyFrameObject_code = 32,         // offsetof(PyFrameObject, f_code)
//.PyFrameObject_lineno = 124,      // offsetof(PyFrameObject, f_lineno)
//.PyFrameObject_localsplus = 376,  // offsetof(PyFrameObject, f_localsplus)
//.PyCodeObject_filename = 96,      // offsetof(PyCodeObject, co_filename)
//.PyCodeObject_name = 104,         // offsetof(PyCodeObject, co_name)
//.PyCodeObject_varnames = 64,      // offsetof(PyCodeObject, co_varnames)
//.PyTupleObject_item = 24,         // offsetof(PyTupleObject, ob_item)
//.String_data = 48,                // sizeof(PyASCIIObject)
//.String_size = 16,                // offsetof(PyVarObject, ob_size)
//};

type OffsetConfig struct {
	PyObject_type            int64
	PyTypeObject_name        int64
	PyThreadState_frame      int64
	PyThreadState_thread     int64
	PyFrameObject_back       int64
	PyFrameObject_code       int64
	PyFrameObject_lineno     int64
	PyFrameObject_localsplus int64
	PyCodeObject_filename    int64
	PyCodeObject_name        int64
	PyCodeObject_varnames    int64
	PyTupleObject_item       int64
	String_data              int64
	String_size              int64
}

var py36OffsetConfig OffsetConfig = OffsetConfig{
	PyObject_type:            8,   // offsetof(PyObject, ob_type)
	PyTypeObject_name:        24,  // offsetof(PyTypeObject, tp_name)
	PyThreadState_frame:      24,  // offsetof(PyThreadState, frame)
	PyThreadState_thread:     152, // offsetof(PyThreadState, thread_id)
	PyFrameObject_back:       24,  // offsetof(PyFrameObject, f_back)
	PyFrameObject_code:       32,  // offsetof(PyFrameObject, f_code)
	PyFrameObject_lineno:     124, // offsetof(PyFrameObject, f_lineno)
	PyFrameObject_localsplus: 376, // offsetof(PyFrameObject, f_localsplus)
	PyCodeObject_filename:    96,  // offsetof(PyCodeObject, co_filename)
	PyCodeObject_name:        104, // offsetof(PyCodeObject, co_name)
	PyCodeObject_varnames:    64,  // offsetof(PyCodeObject, co_varnames)
	PyTupleObject_item:       24,  // offsetof(PyTupleObject, ob_item)
	String_data:              48,  // sizeof(PyASCIIObject)
	String_size:              16,  // offsetof(PyVarObject, ob_size)
}
