// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

package fastdelta

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/spaolacci/murmur3"

	"github.com/grafana/pyroscope/pkg/pproflite"
)

// Hash is a 128-bit hash representing sample identity
type Hash [16]byte

type byHash []Hash

func (h byHash) Len() int           { return len(h) }
func (h byHash) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h byHash) Less(i, j int) bool { return bytes.Compare(h[i][:], h[j][:]) == -1 }

// Hasher ...
type Hasher struct {
	alg murmur3.Hash128
	st  *stringTable
	lx  *locationIndex

	scratch     [8]byte
	labelHashes byHash
	scratchHash Hash
}

// Sample ...
func (h *Hasher) Sample(s *pproflite.Sample) (Hash, error) {
	h.labelHashes = h.labelHashes[:0]
	for i := range s.Label {
		h.labelHashes = append(h.labelHashes, h.label(&s.Label[i]))
	}

	h.alg.Reset()
	for _, id := range s.LocationID {
		addr, ok := h.lx.Get(id)
		if !ok {
			return h.scratchHash, fmt.Errorf("invalid location index")
		}
		binary.LittleEndian.PutUint64(h.scratch[:], addr)
		h.alg.Write(h.scratch[:8])
	}

	// Memory profiles current have exactly one label ("bytes"), so there is no
	// need to sort. This saves ~0.5% of CPU time in our benchmarks.
	if len(h.labelHashes) > 1 {
		sort.Sort(&h.labelHashes) // passing &dc.hashes vs dc.hashes avoids an alloc here
	}

	for _, sub := range h.labelHashes {
		copy(h.scratchHash[:], sub[:]) // avoid sub escape to heap
		h.alg.Write(h.scratchHash[:])
	}
	h.alg.Sum(h.scratchHash[:0])
	return h.scratchHash, nil
}

func (h *Hasher) label(l *pproflite.Label) Hash {
	h.alg.Reset()
	h.alg.Write(h.st.GetBytes(int(l.Key)))
	h.alg.Write(h.st.GetBytes(int(l.NumUnit)))
	binary.BigEndian.PutUint64(h.scratch[:], uint64(l.Num))
	h.alg.Write(h.scratch[0:8])
	h.alg.Write(h.st.GetBytes(int(l.Str)))
	h.alg.Sum(h.scratchHash[:0])
	return h.scratchHash
}
