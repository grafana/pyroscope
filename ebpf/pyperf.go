package ebpfspy

import (
	"bufio"
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
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

func getPythonOffsets(version PythonVersion) (userPyOffsetConfig, error) {
	offsets, ok := pyVersions[version]
	if !ok {
		return offsets, fmt.Errorf("unsupported python version %v ", version)
	}
	return offsets, nil
}

func GetPyPerfPidData(pid int) (*ProfilePyPidData, error) {
	mapsPath := fmt.Sprintf("/proc/%d/maps", pid)

	mapsFD, err := os.Open(mapsPath)
	if err != nil {
		return nil, fmt.Errorf("reading proc maps %s: %w", mapsPath, err)
	}
	defer mapsFD.Close()

	info, err := GetPythonProcInfo(bufio.NewScanner(mapsFD))

	if err != nil {
		return nil, fmt.Errorf("GetPythonProcInfo error %s: %w", mapsPath, err)
	}
	var pythonMeat []*symtab.ProcMap
	if info.LibPythonMaps == nil {
		pythonMeat = info.PythonMaps
	} else {
		pythonMeat = info.LibPythonMaps
	}
	base := pythonMeat[0]
	pythonPath := fmt.Sprintf("/proc/%d/root%s", pid, base.Pathname)
	pythonFD, err := os.Open(pythonPath)
	if err != nil {
		return nil, fmt.Errorf("could not get python patch version %s %w", pythonPath, err)
	}
	defer pythonFD.Close()
	version, err := GetPythonPatchVersion(pythonFD, info.Version)
	if err != nil {
		return nil, fmt.Errorf("could not get python patch version %s %w", pythonPath, err)
	}

	offsets, err := getPythonOffsets(version)

	ef, err := elf.NewFile(pythonFD)
	if err != nil {
		return nil, fmt.Errorf("opening elf %s: %w", pythonPath, err)
	}
	symbols, err := ef.Symbols() // todo reuse parser from symtab, make it optionally streaming
	if err != nil {
		return nil, fmt.Errorf("reading symbols from elf %s: %w", pythonPath, err)
	}

	data := &ProfilePyPidData{}
	var (
		autoTLSkeyAddr, pyRuntimeAddr uint64
	)
	for _, symbol := range symbols {
		switch symbol.Name {
		case "autoTLSkey":
			autoTLSkeyAddr = base.StartAddr + symbol.Value
		case "_PyRuntime":
			pyRuntimeAddr = base.StartAddr + symbol.Value
		default:
			continue
		}
	}
	if pyRuntimeAddr == 0 && autoTLSkeyAddr == 0 {
		return nil, fmt.Errorf("missing symbols pyRuntimeAddr autoTLSkeyAddr %s %v", pythonPath, version)
	}

	data.Version.Major = uint32(version.Major)
	data.Version.Minor = uint32(version.Minor)
	data.Version.Patch = uint32(version.Patch)
	data.TssKey, err = getPythonTSSKey(pid, version, offsets, autoTLSkeyAddr, pyRuntimeAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to get python tss key %w", err)
	}
	if info.Musl != nil {
		muslPath := fmt.Sprintf("/proc/%d/root%s", pid, info.Musl[0].Pathname)

		muslFD, err := os.Open(muslPath)
		if err != nil {
			return nil, fmt.Errorf("couldnot determine musl version %s %w", muslPath, err)
		}
		muslVersion, err := GetMuslVersion(muslFD)
		if err != nil {
			return nil, fmt.Errorf("couldnot determine musl version %s %w", muslPath, err)
		}
		if muslVersion == 0 {
			return nil, fmt.Errorf("couldnot determine musl version %s ", muslPath)
		}

		data.Musl = uint8(muslVersion)
	}
	var vframeCode, vframeBack, vframeLocalPlus int16
	if version.Compare(PythonVersion{3, 11, 0}) >= 0 {
		vframeCode = offsets.PyInterpreterFrame_f_code
		vframeBack = offsets.PyInterpreterFrame_previous
		vframeLocalPlus = offsets.PyInterpreterFrame_localsplus
	} else {
		vframeCode = offsets.PyFrameObject_f_code
		vframeBack = offsets.PyFrameObject_f_back
		vframeLocalPlus = offsets.PyFrameObject_f_localsplus
	}
	if vframeCode == -1 || vframeBack == -1 || vframeLocalPlus == -1 {
		return nil, fmt.Errorf("broken offsets %+v %+v", offsets, version)
	}

	data.Offsets = ProfilePyOffsetConfig{
		PyVarObjectObSize:             offsets.PyVarObject_ob_size,
		PyObjectObType:                offsets.PyObject_ob_type,
		PyTypeObjectTpName:            offsets.PyTypeObject_tp_name,
		PyThreadStateFrame:            offsets.PyThreadState_frame,
		PyThreadStateCframe:           offsets.PyThreadState_cframe,
		PyCFrameCurrentFrame:          offsets.PyCFrame_current_frame,
		PyCodeObjectCoFilename:        offsets.PyCodeObject_co_filename,
		PyCodeObjectCoName:            offsets.PyCodeObject_co_name,
		PyCodeObjectCoVarnames:        offsets.PyCodeObject_co_varnames,
		PyCodeObjectCoLocalsplusnames: offsets.PyCodeObject_co_localsplusnames,
		PyTupleObjectObItem:           offsets.PyTupleObject_ob_item,
		VFrameCode:                    vframeCode,
		VFramePrevious:                vframeBack,
		VFrameLocalsplus:              vframeLocalPlus,
		StringSize:                    offsets.StringSize,
	}
	return data, nil
}

func getPythonTSSKey(pid int, version PythonVersion, offsets userPyOffsetConfig, autoTLSkeyAddr, pyRuntime uint64) (int32, error) {
	fd, err := os.Open(fmt.Sprintf("/proc/%d/mem", pid))
	if err != nil {
		return 0, fmt.Errorf("python memory open failed   %w", err)
	}
	defer fd.Close()
	var key [4]byte
	var pkey int64
	if version.Compare(PythonVersion{3, 7, 0}) < 0 {
		if autoTLSkeyAddr == 0 {
			return 0, fmt.Errorf("python missing symbols autoTLSkey %d %v", pid, version)
		}
		pkey = int64(autoTLSkeyAddr)

	} else {
		if pyRuntime == 0 {
			//should never happen
			return 0, fmt.Errorf("python missing symbols pyRuntime %d %v", pid, version)
		}
		if offsets.PyRuntimeState_gilstate == -1 || offsets.Gilstate_runtime_state_autoTSSkey == -1 || offsets.PyTssT_key == -1 {
			// should never happen
			return 0, fmt.Errorf("python missing offsets PyRuntimeStateGilstate GilstateRuntimeStateAutoTSSkey PyTssT_key %d %v", pid, version)
		}
		pkey = int64(pyRuntime) + int64(offsets.PyRuntimeState_gilstate+offsets.Gilstate_runtime_state_autoTSSkey+offsets.PyTssT_key)
	}

	n, err := fd.ReadAt(key[:], int64(pkey))
	if err != nil {
		return 0, fmt.Errorf("python failed to read key %d %d %v %w", pid, pkey, version, err)
	}
	if n != 4 {
		return 0, fmt.Errorf("python failed to read key %d %d %v %w", pid, pkey, version, err)
	}
	res := int32(binary.LittleEndian.Uint32(key[:]))
	fmt.Printf("tssKey %x = %d\n", pkey, res)
	return res, nil
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
			res[k] = fmt.Sprintf("%s!%s", strFromInt8(keys[i].Name[:]), strFromInt8(keys[i].File[:]))
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

type PythonProcInfo struct {
	Version       PythonVersion
	PythonMaps    []*symtab.ProcMap
	LibPythonMaps []*symtab.ProcMap
	Musl          []*symtab.ProcMap
}

var rePython = regexp.MustCompile("/.*/((?:lib)?python)(\\d+)\\.(\\d+)(?:[mu]?\\.so)?(?:.1.0)?$")

func GetPythonProcInfo(s *bufio.Scanner) (PythonProcInfo, error) {
	res := PythonProcInfo{}
	i := 0
	for s.Scan() {
		line := s.Bytes()
		m, err := symtab.ParseProcMapLine(line, false)
		if err != nil {
			return res, err
		}
		if m.Pathname != "" {
			matches := rePython.FindAllStringSubmatch(m.Pathname, -1)
			if matches != nil {
				if res.Version.Major == 0 {
					maj, err := strconv.Atoi(matches[0][2])
					if err != nil {
						return res, fmt.Errorf("failed to parse python version %s", m.Pathname)
					}
					min, err := strconv.Atoi(matches[0][3])
					if err != nil {
						return res, fmt.Errorf("failed to parse python version %s", m.Pathname)
					}
					res.Version.Major = maj
					res.Version.Minor = min
				}
				typ := matches[0][1]
				if typ == "python" {
					res.PythonMaps = append(res.PythonMaps, m)
				} else {
					res.LibPythonMaps = append(res.LibPythonMaps, m)
				}

				i += 1
			}
			if strings.Contains(m.Pathname, "/lib/ld-musl-x86_64.so.1") {
				res.Musl = append(res.Musl, m)
			}
		}
	}
	if res.LibPythonMaps == nil && res.PythonMaps == nil {
		return res, fmt.Errorf("no python found")
	}
	return res, nil
}
