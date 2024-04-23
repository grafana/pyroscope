package python

import (
	"fmt"
	"testing"
)

func TestDump(t *testing.T) {

	for v, o := range pyVersions {
		fmt.Printf("// %d.%d.%d \n", v.Major, v.Minor, v.Patch)
		fmt.Printf("PyVarObject_ob_size: %d,\n", o.PyVarObject_ob_size)
		fmt.Printf("PyObject_ob_type: %d,\n", o.PyObject_ob_type)
		fmt.Printf("PyTypeObject_tp_name: %d,\n", o.PyTypeObject_tp_name)
		fmt.Printf("PyThreadState_frame: %d,\n", o.PyThreadState_frame)
		fmt.Printf("PyThreadState_cframe: %d,\n", o.PyThreadState_cframe)
		fmt.Printf("PyThreadState_current_frame: %d,\n", o.PyThreadState_current_frame)
		fmt.Printf("PyCFrame_current_frame: %d,\n", o.PyCFrame_current_frame)
		fmt.Printf("PyFrameObject_f_back: %d,\n", o.PyFrameObject_f_back)
		fmt.Printf("PyFrameObject_f_code: %d,\n", o.PyFrameObject_f_code)
		fmt.Printf("PyFrameObject_f_localsplus: %d,\n", o.PyFrameObject_f_localsplus)
		fmt.Printf("PyCodeObject_co_filename: %d,\n", o.PyCodeObject_co_filename)
		fmt.Printf("PyCodeObject_co_name: %d,\n", o.PyCodeObject_co_name)
		fmt.Printf("PyCodeObject_co_varnames: %d,\n", o.PyCodeObject_co_varnames)
		fmt.Printf("PyCodeObject_co_localsplusnames: %d,\n", o.PyCodeObject_co_localsplusnames)
		fmt.Printf("PyTupleObject_ob_item: %d,\n", o.PyTupleObject_ob_item)
		fmt.Printf("PyInterpreterFrame_f_code: %d,\n", o.PyInterpreterFrame_f_code)
		fmt.Printf("PyInterpreterFrame_f_executable: %d,\n", o.PyInterpreterFrame_f_executable)
		fmt.Printf("PyInterpreterFrame_previous: %d,\n", o.PyInterpreterFrame_previous)
		fmt.Printf("PyInterpreterFrame_localsplus: %d,\n", o.PyInterpreterFrame_localsplus)
		fmt.Printf("PyInterpreterFrame_owner: %d,\n", o.PyInterpreterFrame_owner)
		fmt.Printf("PyRuntimeState_gilstate: %d,\n", o.PyRuntimeState_gilstate)
		fmt.Printf("PyRuntimeState_autoTSSkey: %d,\n", o.PyRuntimeState_autoTSSkey)
		fmt.Printf("Gilstate_runtime_state_autoTSSkey: %d,\n", o.Gilstate_runtime_state_autoTSSkey)
		fmt.Printf("PyTssT_is_initialized: %d,\n", o.PyTssT_is_initialized)
		fmt.Printf("PyTssT_key: %d,\n", o.PyTssT_key)
		fmt.Printf("PyTssTSize: %d,\n", o.PyTssTSize)
		fmt.Printf("PyASCIIObjectSize: %d,\n", o.PyASCIIObjectSize)
		fmt.Printf("PyCompactUnicodeObjectSize: %d,\n", o.PyCompactUnicodeObjectSize)
	}
}
