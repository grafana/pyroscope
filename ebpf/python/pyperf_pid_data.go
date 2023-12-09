package python

import (
	"debug/elf"
	"fmt"
	"os"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/pyroscope/ebpf/symtab"
)

type PySymbols struct {
	autoTLSkeyAddr uint64
	pyRuntimeAddr  uint64

	PyObject_Malloc_Impl, PyObject_Calloc_Impl, PyObject_Realloc_Impl, PyObject_Free_Impl uint64
	PyObject_Free                                                                         uint64
}

func getSymbols(pythonFD *os.File, base *symtab.ProcMap, flags GetProcDataFlags) (*PySymbols, error) {
	ef, err := elf.NewFile(pythonFD)
	if err != nil {
		return nil, fmt.Errorf("opening elf %w", err)
	}
	res := new(PySymbols)
	baseAddr := base.StartAddr
	if ef.FileHeader.Type == elf.ET_EXEC {
		baseAddr = 0
	}
	symbols, err := ef.DynamicSymbols()
	if err != nil {
		return nil, fmt.Errorf("reading symbols from elf %w", err)
	}
	for _, symbol := range symbols {
		switch symbol.Name {
		case "autoTLSkey":
			res.autoTLSkeyAddr = baseAddr + symbol.Value
		case "_PyRuntime":
			res.pyRuntimeAddr = baseAddr + symbol.Value
		case "PyObject_Free":
			res.PyObject_Free = baseAddr + symbol.Value
		default:
			continue
		}
	}
	if res.pyRuntimeAddr == 0 && res.autoTLSkeyAddr == 0 {
		return nil, fmt.Errorf("missing symbols %+v ", symbols)
	}
	needMem := flags&GetProcDataFlagWithMem != 0
	if needMem {
		symbols, err = ef.Symbols()
		_ = err // Ignore. The memory sampling just will not work, but cpu still will
		for _, symbol := range symbols {
			switch symbol.Name {
			case "_PyObject_Malloc":
				res.PyObject_Malloc_Impl = baseAddr + symbol.Value
			case "_PyObject_Calloc":
				res.PyObject_Calloc_Impl = baseAddr + symbol.Value
			case "_PyObject_Realloc":
				res.PyObject_Realloc_Impl = baseAddr + symbol.Value
			case "_PyObject_Free":
				res.PyObject_Free_Impl = baseAddr + symbol.Value
			}
		}
	}

	return res, nil
}

type GetProcDataFlags int

var (
	GetProcDataFlagWithMem = GetProcDataFlags(1)
)

type ProcData struct {
	PID      int
	ProcInfo *ProcInfo
	// data passed to ebpf program
	PerfPyPidData *PerfPyPidData
	// addresses in VM of the process
	PySymbols *PySymbols

	Base *symtab.ProcMap
}

func GetProcData(l log.Logger, info *ProcInfo, pid uint32, flags GetProcDataFlags) (*ProcData, error) {

	pythonMeat := getPythonMaps(info)
	readable := findReadableMap(pythonMeat)
	if readable == nil {
		return nil, fmt.Errorf("no readable python map entry %+v", pythonMeat)
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

	symbols, err := getSymbols(pythonFD, base_, flags)
	if err != nil {
		return nil, fmt.Errorf("could not get python symbols %s %w", pythonPath, err)
	}

	data := &PerfPyPidData{}

	data.Version.Major = uint32(version.Major)
	data.Version.Minor = uint32(version.Minor)
	data.Version.Patch = uint32(version.Patch)
	data.Libc, err = GetLibc(l, pid, info)
	if err != nil {
		return nil, fmt.Errorf("failed to get python process libc %w", err)
	}
	data.TssKey, err = GetTSSKey(pid, version, offsets, symbols, &data.Libc)
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
		AddressZeroSymbol:             readable.StartAddr,
	}
	return &ProcData{
		PID:           int(pid),
		PerfPyPidData: data,
		PySymbols:     symbols,
		ProcInfo:      info,
		Base:          base_,
	}, nil
}

// return a map entry that is readable and not writable
func findReadableMap(pythonMeat []*symtab.ProcMap) *symtab.ProcMap {
	var readable *symtab.ProcMap
	for _, m := range pythonMeat {
		if m.Perms.Read && !m.Perms.Write {
			readable = m
			break
		}
	}
	return readable
}

func getPythonMaps(info *ProcInfo) []*symtab.ProcMap {
	if info.LibPythonMaps == nil {
		return info.PythonMaps
	}
	return info.LibPythonMaps
}
