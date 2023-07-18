package python

import (
	"encoding/binary"
	"fmt"
	"os"
)

func GetTSSKey(pid uint32, version Version, offsets *UserOffsets, autoTLSkeyAddr, pyRuntime uint64) (int32, error) {
	fd, err := os.Open(fmt.Sprintf("/proc/%d/mem", pid))
	if err != nil {
		return 0, fmt.Errorf("python memory open failed   %w", err)
	}
	defer fd.Close()
	var key [4]byte
	var pkey int64
	if version.Compare(Py37) < 0 {
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
	return res, nil
}
