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
	if version.Compare(Py37) < 0 {
		return getAutoTLSKey(pid, version, autoTLSkeyAddr, fd)
	} else {
		return getPyTssKey(pid, version, offsets, pyRuntime, fd)
	}
}

func getAutoTLSKey(pid uint32, version Version, autoTLSkeyAddr uint64, mem *os.File) (int32, error) {
	var pkey int64
	var key [4]byte
	if autoTLSkeyAddr == 0 {
		return 0, fmt.Errorf("python missing symbols autoTLSkey %d %v", pid, version)
	}
	pkey = int64(autoTLSkeyAddr)

	n, err := mem.ReadAt(key[:], int64(pkey))
	if err != nil {
		return 0, fmt.Errorf("python failed to read key %d %d %v %w", pid, pkey, version, err)
	}
	if n != 4 {
		return 0, fmt.Errorf("python failed to read key %d %d %v %w", pid, pkey, version, err)
	}
	res := int32(binary.LittleEndian.Uint32(key[:]))
	if res == -1 {
		return 0, fmt.Errorf("python not initialized %+v", version)
	}
	return res, nil
}

func getPyTssKey(pid uint32, version Version, offsets *UserOffsets, pyRuntime uint64, mem *os.File) (int32, error) {
	if offsets.PyTssT_is_initialized != 0 || offsets.PyTssT_key != 4 || offsets.PyTssTSize != 8 {
		return 0, fmt.Errorf("unexpected _Py_tss_t offsets %+v %+v", offsets, version)
	}
	var pkey int64
	var key [8]byte
	if pyRuntime == 0 {
		//should never happen
		return 0, fmt.Errorf("python missing symbols pyRuntime %d %v", pid, version)
	}
	if offsets.PyRuntimeState_gilstate == -1 || offsets.Gilstate_runtime_state_autoTSSkey == -1 || offsets.PyTssT_key == -1 {
		// should never happen
		return 0, fmt.Errorf("python missing offsets PyRuntimeStateGilstate GilstateRuntimeStateAutoTSSkey PyTssT_key %d %v", pid, version)
	}
	pkey = int64(pyRuntime) + int64(offsets.PyRuntimeState_gilstate+offsets.Gilstate_runtime_state_autoTSSkey+offsets.PyTssT_key)
	n, err := mem.ReadAt(key[:], int64(pkey))
	if err != nil {
		return 0, fmt.Errorf("python failed to read key %d %d %v %w", pid, pkey, version, err)
	}
	if n != 8 {
		return 0, fmt.Errorf("python failed to read key %d %d %v %w", pid, pkey, version, err)
	}
	isInitialized := int32(binary.LittleEndian.Uint32(key[:4]))
	res := int32(binary.LittleEndian.Uint32(key[4:8]))
	if isInitialized == 0 || res == -1 {
		return 0, fmt.Errorf("python not initialized %+v", version)
	}
	return res, nil
}
