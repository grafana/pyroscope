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

func GetPyPerfPidData(l log.Logger, pid uint32, collectKernel bool) (*PerfPyPidData, error) {
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
	base_ := pythonMeat[0]
	pythonPath := fmt.Sprintf("/proc/%d/root%s", pid, base_.Pathname)
	pythonFD, err := os.Open(pythonPath)
	if err != nil {
		return nil, fmt.Errorf("could not open python path %s %w", pythonPath, err)
	}
	defer pythonFD.Close()
	version, err := GetPythonPatchVersion(pythonFD, info.Version)
	if err != nil {
		return nil, fmt.Errorf("could not get python patch version %s %w", pythonPath, err)
	}

	offsets, guess, err := GetUserOffsets(version)
	if err != nil {
		return nil, fmt.Errorf("unsupported python version %w %+v", err, version)
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
	baseAddr := base_.StartAddr
	if ef.FileHeader.Type == elf.ET_EXEC {
		baseAddr = 0
	}
	for _, symbol := range symbols {
		switch symbol.Name {
		case "autoTLSkey":
			autoTLSkeyAddr = baseAddr + symbol.Value
		case "_PyRuntime":
			pyRuntimeAddr = baseAddr + symbol.Value
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
	data.Libc, err = GetLibc(l, pid, info)
	if err != nil {
		return nil, fmt.Errorf("failed to get python process libc %w", err)
	}
	data.TssKey, err = GetTSSKey(pid, version, offsets, autoTLSkeyAddr, pyRuntimeAddr, &data.Libc)
	if err != nil {
		return nil, fmt.Errorf("failed to get python tss key %w", err)
	}

	var vframeCode, vframeBack, vframeLocalPlus int16
	if version.Compare(Py311) >= 0 {
		if version.Compare(Py313) >= 0 {
			vframeCode = offsets.PyInterpreterFrame_f_executable
		} else {
			vframeCode = offsets.PyInterpreterFrame_f_code
		}
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

	cframe := offsets.PyThreadState_cframe
	currentFrame := offsets.PyCFrame_current_frame
	frame := offsets.PyThreadState_frame
	if version.Compare(Py313) >= 0 {
		if cframe != -1 || currentFrame != -1 || frame != -1 {
			return nil, fmt.Errorf("broken offsets %+v %+v", offsets, version)
		}
		// PyCFrame was removed in 3.13, lets pretend it was never there and frame field was just renamed to current_frame
		frame = offsets.PyThreadState_current_frame
		if frame == -1 {
			return nil, fmt.Errorf("broken offsets %+v %+v", offsets, version)
		}
	}

	data.Offsets = PerfPyOffsetConfig{
		PyThreadStateFrame:            frame,
		PyThreadStateCframe:           cframe,
		PyCFrameCurrentFrame:          currentFrame,
		PyCodeObjectCoFilename:        offsets.PyCodeObject_co_filename,
		PyCodeObjectCoName:            offsets.PyCodeObject_co_name,
		PyCodeObjectCoVarnames:        offsets.PyCodeObject_co_varnames,
		PyCodeObjectCoLocalsplusnames: offsets.PyCodeObject_co_localsplusnames,
		PyTupleObjectObItem:           offsets.PyTupleObject_ob_item,
		VFrameCode:                    vframeCode,
		VFramePrevious:                vframeBack,
		VFrameLocalsplus:              vframeLocalPlus,
		PyInterpreterFrameOwner:       offsets.PyInterpreterFrame_owner,
		PyASCIIObjectSize:             offsets.PyASCIIObjectSize,
		PyCompactUnicodeObjectSize:    offsets.PyCompactUnicodeObjectSize,
		PyVarObjectObSize:             offsets.PyVarObject_ob_size,
		PyObjectObType:                offsets.PyObject_ob_type,
		PyTypeObjectTpName:            offsets.PyTypeObject_tp_name,
	}
	if collectKernel {
		data.CollectKernel = 1
	} else {
		data.CollectKernel = 0
	}
	return data, nil
}
