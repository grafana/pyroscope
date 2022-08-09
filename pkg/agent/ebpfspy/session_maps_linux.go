//go:build ebpfspy
// +build ebpfspy

// Package ebpfspy provides integration with Linux eBPF. It is a rough copy of profile.py from BCC tools:
//   https://github.com/iovisor/bcc/blob/master/tools/profile.py
package ebpfspy

import "C"
import (
	"fmt"
	bpf "github.com/aquasecurity/libbpfgo"
	"unsafe"
)

//#cgo CFLAGS: -I./bpf/
//#include <linux/types.h>
//#include "profile.bpf.h"
import "C"

func (s *session) getCountsMapValues() (keys [][]byte, values [][]byte, batch bool, err error) {
	// try lookup_and_delete_batch
	var (
		mapSize = C.PROFILE_MAPS_SIZE
		keySize = int(unsafe.Sizeof(C.struct_profile_key_t{}))
		allKeys = make([]byte, mapSize*keySize)
		pKeys   = unsafe.Pointer(&allKeys[0])
		nextKey = C.struct_profile_key_t{}
	)
	values, err = s.mapCounts.GetValueAndDeleteBatch(pKeys, nil, unsafe.Pointer(&nextKey), uint32(mapSize))
	if len(values) > 0 {
		keys = collectBatchValues(allKeys, len(values), keySize)
		return keys, values, true, nil
	}
	// batch failed or unsupported or just unlucky and got 0 stack-traces
	// try iterating
	it := s.mapCounts.Iterator()
	for it.Next() {
		k := it.Key()
		allKeys = append(allKeys, k...)
		ck := (*C.struct_profile_key_t)(unsafe.Pointer(&k[0]))
		v, err := s.mapCounts.GetValue(unsafe.Pointer(ck))
		if err != nil {
			return nil, nil, false, err
		}
		keys = append(keys, k)
		values = append(keys, v)
	}
	return keys, values, false, nil
}

func (s *session) clearCountsMap(keys [][]byte, batch bool) error {
	fmt.Println("clearCountsMap", len(keys))
	if len(keys) == 0 {
		return nil
	}
	if batch {
		// do nothing, already deleted with GetValueAndDeleteBatch in getCountsMapValues
		fmt.Println("doing nothing, already deleted")
		return nil
	}
	fmt.Println("deleting iter")
	for _, key := range keys {
		err := s.mapCounts.DeleteKey(unsafe.Pointer(&key[0]))
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *session) clearStacksMap(knownKeys map[uint32]bool) error {
	m := s.mapStacks
	if s.roundNumber%3 == 0 { //todo increase, 3- for debugging
		fmt.Printf("do a full stackmap reset")
		// do a full reset once in a while
		it := m.Iterator()
		for it.Next() {

			if err := m.DeleteKey(unsafe.Pointer(&it.Key()[0])); err != nil {
				return err
			}
		}
		return nil
	}
	fmt.Printf("do a known keys stackmap reset")

	for stackId := range knownKeys {
		if err := m.DeleteKey(unsafe.Pointer(&stackId)); err != nil {
			return err
		}
	}
	return nil
}

func clearMap(m *bpf.BPFMap) error {
	it := m.Iterator()
	for it.Next() {
		err := m.DeleteKey(unsafe.Pointer(&it.Key()[0]))
		if err != nil {
			return err
		}
	}
	return nil
}

func collectBatchValues(values []byte, count int, valueSize int) [][]byte {
	var value []byte
	var collected [][]byte
	for i := 0; i < count*valueSize; i += valueSize {
		value = values[i : i+valueSize]
		collected = append(collected, value)
	}
	return collected
}
