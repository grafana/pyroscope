// Package hyperloglog wraps github.com/clarkduvall/hyperloglog with mutexes
package hyperloglog

import (
	"sync"

	original "github.com/clarkduvall/hyperloglog"
)

type Hash64 original.Hash64

type HyperLogLogPlus struct {
	hMutex sync.Mutex
	h      *original.HyperLogLogPlus
}

func NewPlus(n uint8) (*HyperLogLogPlus, error) {
	h, err := original.NewPlus(n)
	if err != nil {
		return nil, err
	}

	return &HyperLogLogPlus{
		h: h,
	}, nil
}

func (h *HyperLogLogPlus) Count() uint64 {
	h.hMutex.Lock()
	defer h.hMutex.Unlock()

	return h.h.Count()
}

func (h *HyperLogLogPlus) Add(s Hash64) {
	h.hMutex.Lock()
	defer h.hMutex.Unlock()

	h.h.Add(s)
}
