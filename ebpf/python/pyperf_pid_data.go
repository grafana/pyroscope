package python

import (
	"bufio"
	"debug/elf"
	"fmt"
	"os"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/pyroscope/ebpf/symtab"
)

func GetPyPerfPidData(l log.Logger, pid uint32) (*PerfPyPidData, error) {
	mapsFD, err := os.Open(fmt.Sprintf("/proc/%d/maps", pid))
	if err != nil {
		return nil, fmt.Errorf("reading proc maps %d: %w", pid, err)
	}
	defer mapsFD.Close()

	info, err := GetProcInfo(bufio.NewScanner(mapsFD))

	if err != nil {
		return nil, fmt.Errorf("GetPythonProcInfo error %s: %w", fmt.Sprintf("/proc/%d/maps", pid), err)
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

	offsets, guess, err := GetUserOffsets(version)
	if err != nil {
		return nil, fmt.Errorf("unsupported python version %+v", version)
	}
	if guess {
		level.Warn(l).Log("msg", "python offsets were not found, but guessed from the closest patch version")
	}

	ef, err := elf.NewFile(pythonFD)
	if err != nil {
		return nil, fmt.Errorf("opening elf %s: %w", pythonPath, err)
	}
	symbols, err := ef.DynamicSymbols()
	if err != nil {
		return nil, fmt.Errorf("reading symbols from elf %s: %w", pythonPath, err)
	}

	data := &PerfPyPidData{}
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
	data.TssKey, err = GetTSSKey(pid, version, offsets, autoTLSkeyAddr, pyRuntimeAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to get python tss key %w", err)
	}
	if info.Musl != nil {
		muslPath := fmt.Sprintf("/proc/%d/root%s", pid, info.Musl[0].Pathname)
		muslVersion, err := GetMuslVersionFromFile(muslPath)
		if err != nil {
			return nil, fmt.Errorf("couldnot determine musl version %s %w", muslPath, err)
		}
		data.Musl = uint8(muslVersion)
	}
	var vframeCode, vframeBack, vframeLocalPlus int16
	if version.Compare(Py311) >= 0 {
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

	data.Offsets = PerfPyOffsetConfig{
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
	}
	return data, nil
}
