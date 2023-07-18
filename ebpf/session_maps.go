//go:build linux

// Package ebpfspy provides integration with Linux eBPF. It is a rough copy of profile.py from BCC tools:
//
//	https://github.com/iovisor/bcc/blob/master/tools/profile.py
package ebpfspy

import (
	"fmt"

	"github.com/cilium/ebpf"
	"github.com/go-kit/log/level"
)

func (s *session) getCountsMapValues() (keys []profileSampleKey, values []uint32, batch bool, err error) {
	// try batch first
	var (
		m       = s.bpf.profileMaps.Counts
		mapSize = m.MaxEntries()
		nextKey = profileSampleKey{}
	)
	keys = make([]profileSampleKey, mapSize)
	values = make([]uint32, mapSize)

	opts := &ebpf.BatchOptions{}
	n, _ := m.BatchLookupAndDelete(nil, &nextKey, keys, values, opts)
	if n > 0 {
		level.Debug(s.logger).Log(
			"msg", "getCountsMapValues BatchLookupAndDelete",
			"count", n,
		)
		return keys[:n], values[:n], true, nil
	}
	// try iterating if batch failed
	resultKeys := keys[:0]
	resultValues := values[:0]
	it := m.Iterate()
	k := profileSampleKey{}
	v := uint32(0)
	for {
		ok := it.Next(&k, &v)
		if !ok {
			err := it.Err()
			if err != nil {
				err = fmt.Errorf("map %s iteration : %w", m.String(), err)
				return nil, nil, false, err
			}
			break
		}
		resultKeys = append(resultKeys, k)
		resultValues = append(resultValues, v)
	}
	level.Debug(s.logger).Log(
		"msg", "getCountsMapValues iter",
		"count", len(keys),
	)
	return resultKeys, resultValues, false, nil
}

func (s *session) clearCountsMap(keys []profileSampleKey, batch bool) error {
	if len(keys) == 0 {
		return nil
	}
	if batch {
		// do nothing, already deleted with GetValueAndDeleteBatch in getCountsMapValues
		return nil
	}
	m := s.bpf.profileMaps.Counts
	for i := range keys {
		err := m.Delete(&keys[i])
		if err != nil {
			return err
		}
	}
	level.Debug(s.logger).Log(
		"msg", "clearCountsMap",
		"count", len(keys),
	)
	return nil
}

func (s *session) clearStacksMap(knownKeys map[uint32]bool) error {
	m := s.bpf.Stacks
	cnt := 0
	errs := 0
	if s.roundNumber%10 == 0 {
		// do a full reset once in a while
		it := m.Iterate()
		v := make([]byte, m.ValueSize())
		var keys []uint32
		for {
			k := uint32(0)
			ok := it.Next(&k, &v)
			if !ok {
				err := it.Err()
				if err != nil {
					return fmt.Errorf("clearStacksMap fail: %w", err)
				}
				break
			}
			keys = append(keys, k)
		}
		for i := range keys {
			if err := m.Delete(&keys[i]); err != nil {
				errs += 1
			} else {
				cnt += 1
			}
		}
		level.Debug(s.logger).Log(
			"msg", "clearStacksMap deleted all stacks",
			"count", cnt,
			"unsuccessful", errs,
		)
		return nil
	}
	for stackId := range knownKeys {
		k := stackId
		if err := m.Delete(&k); err != nil {
			errs += 1
		} else {
			cnt += 1
		}
	}
	level.Debug(s.logger).Log(
		"msg", "clearStacksMap deleted known stacks",
		"count", cnt,
		"unsuccessful", errs,
	)
	return nil
}
