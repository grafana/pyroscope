//go:build ebpfspy

// Package ebpfspy provides integration with Linux eBPF. It is a rough copy of profile.py from BCC tools:
//
//	https://github.com/iovisor/bcc/blob/master/tools/profile.py
package ebpfspy

import (
	"unsafe"
)

//#cgo CFLAGS: -I./bpf/
//#include <linux/types.h>
//#include "profile.bpf.h"
import "C"

func (s *Session) getCountsMapValues() (keys [][]byte, values [][]byte, batch bool, err error) {
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
		key := it.Key()
		v, err := s.mapCounts.GetValue(unsafe.Pointer(&key[0]))
		if err != nil {
			return nil, nil, false, err
		}
		keyCopy := make([]byte, len(key)) // The slice is valid only until the next call to Next.
		copy(keyCopy, key)
		keys = append(keys, keyCopy)
		values = append(values, v)
	}
	return keys, values, false, nil
}

func (s *Session) clearCountsMap(keys [][]byte, batch bool) error {
	if len(keys) == 0 {
		return nil
	}
	if batch {
		// do nothing, already deleted with GetValueAndDeleteBatch in getCountsMapValues
		return nil
	}
	for _, key := range keys {
		err := s.mapCounts.DeleteKey(unsafe.Pointer(&key[0]))
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Session) clearStacksMap(knownKeys map[uint32]bool) error {
	m := s.mapStacks
	cnt := 0
	errs := 0
	if s.roundNumber%10 == 0 {
		// do a full reset once in a while
		it := m.Iterator()
		var keys [][]byte
		for it.Next() {
			key := it.Key()
			keyCopy := make([]byte, len(key)) // The slice is valid only until the next call to Next.
			copy(keyCopy, key)
			keys = append(keys, keyCopy)
		}
		for _, key := range keys {
			if err := m.DeleteKey(unsafe.Pointer(&key[0])); err != nil {
				errs += 1
			} else {
				cnt += 1
			}
		}
		return nil
	}
	for stackId := range knownKeys {
		k := stackId
		if err := m.DeleteKey(unsafe.Pointer(&k)); err != nil {
			errs += 1
		} else {
			cnt += 1
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
