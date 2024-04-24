package python

import (
	"bufio"
	"debug/elf"
	"fmt"
	"os"
	"reflect"

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
		typecheck                     PerfPyTypecheckData
	)
	baseAddr := base_.StartAddr
	if ef.FileHeader.Type == elf.ET_EXEC {
		baseAddr = 0
	}
	symbolsBind := map[string]*uint64{}
	bind := func(name string, addr *uint64) {
		symbolsBind[name] = addr
	}
	bind("autoTLSkey", &autoTLSkeyAddr)
	bind("_PyRuntime", &pyRuntimeAddr)
	bind("PyCode_Type", &typecheck.PyCodeType)
	bind("PyFrame_Type", &typecheck.PyFrameType)
	bind("PyBytes_Type", &typecheck.PyBytesType)
	bind("PyUnicode_Type", &typecheck.PyUnicodeType)
	bind("PyType_Type", &typecheck.PyTypeType)
	bind("PyDict_Type", &typecheck.PyDictType)
	bind("_PyNone_Type", &typecheck.PyNoneType)
	bind("PyModule_Type", &typecheck.PyModuleType)

	for _, symbol := range symbols {
		if addr, ok := symbolsBind[symbol.Name]; ok {
			*addr = baseAddr + symbol.Value
		}
	}
	if pyRuntimeAddr == 0 && autoTLSkeyAddr == 0 {
		return nil, fmt.Errorf("missing symbols pyRuntimeAddr autoTLSkeyAddr %s %v", pythonPath, version)
	}
	typecheck.O_PyThreadStateDict = uint64(offsets.PyThreadState_dict)
	typecheck.O_PyThreadStateInterp = uint64(offsets.PyThreadState_interp)
	typecheck.SizePyThreadState = uint64(offsets.PyThreadStateSize)
	typecheck.O_PyInterpreterStateTstateHead = uint64(offsets.PyInterpreterState_tstate_head)
	typecheck.O_PyInterpreterStateFinalizing = uint64(offsets.PyInterpreterState_finalizing)
	typecheck.O_PyInterpreterStateModules = uint64(offsets.PyInterpreterState_modules)
	typecheck.O_PyInterpreterStateImportlib = uint64(offsets.PyInterpreterState_importlib)
	typecheck.SizePyInterpreterStateTstate = uint64(offsets.PyInterpreterStateSize)

	if err := validateTypeCheck(typecheck); err != nil {
		return nil, fmt.Errorf("failed to validate typecheck %w", err)
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
	if data.TssKey != 0 {
		level.Warn(l).Log("msg", "tss key is not 0", "tss key", data.TssKey)
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
	data.Typecheck = typecheck
	if collectKernel {
		data.CollectKernel = 1
	} else {
		data.CollectKernel = 0
	}
	return data, nil
}

func validateTypeCheck(tc PerfPyTypecheckData) error {
	v := reflect.ValueOf(tc)
	for i := 0; i < v.NumField(); i++ {
		name := v.Type().Field(i).Name
		vv := uint64(v.Field(i).Uint())
		fmt.Printf("tc %s %v\n", name, vv)
		i2 := int64(-1)
		if vv == 0 || vv == uint64(i2) {
			return fmt.Errorf("field %s is not found", name)
		}
	}
	return nil
}
