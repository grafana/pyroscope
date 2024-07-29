// MIT License
//
// Copyright (c) 2022 Twilio, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package heavykeeper

import (
	"container/heap"
	"math"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/OneOfOne/xxhash"
)

func New() *HeavyKeeper { return NewHeavyKeeper(1<<10, 0.9, time.Minute) }

// HeavyKeeper implements the Top-K algorithm described in "HeavyKeeper: An
// Accurate Algorithm for Finding Top-k Elephant Flows" at
// https://www.usenix.org/system/files/conference/atc18/atc18-gong.pdf
//
// HeavyKeeper is not safe for concurrent use.
type HeavyKeeper struct {
	mu sync.Mutex

	decay   float64
	buckets [][]bucket
	heap    minHeap

	window float64
	res    uint32
}

type bucket struct {
	fingerprint uint32
	emwa        uint32
	last        int64
	cur         uint32
}

func NewHeavyKeeper(k int, decay float64, window time.Duration) *HeavyKeeper {
	if k < 1 {
		panic("k must be >= 1")
	}

	if decay <= 0 || decay > 1 {
		panic("decay must be in range (0, 1.0]")
	}

	width := int(float64(k) * math.Log(float64(k)))
	if width < 256 {
		width = 256
	}

	depth := int(math.Log(float64(k)))
	if depth < 3 {
		depth = 3
	}

	buckets := make([][]bucket, depth)
	for i := range buckets {
		buckets[i] = make([]bucket, width)
	}

	return &HeavyKeeper{
		decay:   decay,
		buckets: buckets,
		heap:    make(minHeap, k),
		window:  float64(window),
		res:     uint32(window / time.Second),
	}
}

func emwa(value, old, delta, window float64) float64 {
	alpha := 1.0 - math.Exp(-delta/window)
	return alpha*value + (1.0-alpha)*old
}

// Sample increments the given flow's count by the given amount. It returns
// true if the flow is in the top K elements.
func (hk *HeavyKeeper) Sample(flow string, incr uint32, now int64) (fc FlowCount, found bool) {
	hk.mu.Lock()
	defer hk.mu.Unlock()

	fp := fingerprint(flow)
	var maxCount uint32
	heapMin := hk.heap.Min()

	for i, row := range hk.buckets {
		j := slot(flow, uint32(i), uint32(len(row)))

		if row[j].emwa == 0 {
			row[j].fingerprint = fp
			row[j].emwa = incr
			row[j].cur = incr
			row[j].last = now
			maxCount = max(maxCount, incr)
		} else if row[j].fingerprint == fp {
			row[j].cur += incr
			delta := now - row[j].last
			if delta > int64(time.Second) {
				row[j].emwa = uint32(emwa(float64(row[j].cur), float64(row[j].emwa), float64(delta), hk.window))
				row[j].last = now
				row[j].cur = 0
			}
			maxCount = max(maxCount, row[j].emwa)
		} else {
			if rand.Float64() < math.Pow(hk.decay, float64(row[j].emwa)) {
				row[j].emwa -= incr
				if row[j].emwa <= 0 {
					row[j].fingerprint = fp
					row[j].emwa = incr
					row[j].cur = incr
					row[j].last = now
					maxCount = max(maxCount, incr)
					break
				}
			}
		}
	}

	if maxCount >= heapMin {
		i := hk.heap.Find(flow)
		if i < 0 {
			i = 0
			hk.heap[0].Flow = strings.Clone(flow)
		}
		hk.heap[i].Count = maxCount
		heap.Fix(&hk.heap, i)
		return hk.heap[i], true
	}

	return fc, false
}

func fingerprint(flow string) uint32 {
	return xxhash.ChecksumString32S(flow, math.MaxUint32)
}

func slot(flow string, row, width uint32) uint32 {
	return xxhash.ChecksumString32S(flow, row) % width
}

// FlowCount is a tuple of flow and estimated count.
type FlowCount struct {
	Flow  string
	Count uint32
}

type byCount []FlowCount

func (a byCount) Len() int           { return len(a) }
func (a byCount) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byCount) Less(i, j int) bool { return a[i].Count < a[j].Count }

func (hk *HeavyKeeper) Top() []FlowCount {
	top := make([]FlowCount, len(hk.heap))
	copy(top, hk.heap)
	sort.Stable(sort.Reverse(byCount(top)))

	// Trim off empty values
	end := len(top)
	for ; end > 0; end-- {
		if top[end-1].Count > 0 {
			break
		}
	}

	return top[:end]
}

func (hk *HeavyKeeper) Rate(flow string) (uint32, bool) {
	hk.mu.Lock()
	defer hk.mu.Unlock()
	for _, hb := range hk.heap {
		if hb.Flow == flow {
			return hb.Count / hk.res, true
		}
	}
	return 0, false
}

// DecayAll decays all flows by the given percentage.
func (hk *HeavyKeeper) DecayAll(pct float64) {
	if pct <= 0 {
		return
	} else if pct > 1 {
		hk.Reset()
		return
	}

	pct = 1 - pct

	for _, row := range hk.buckets {
		for i := range row {
			row[i].emwa = uint32(float64(row[i].emwa) * pct)
		}
	}
	for i := range hk.heap {
		hk.heap[i].Count = uint32(float64(hk.heap[i].Count) * pct)
	}
}

// Reset returns the HeavyKeeper to a like-new state with no flows and no
// counts.
func (hk *HeavyKeeper) Reset() {
	for _, row := range hk.buckets {
		for i := range row {
			row[i] = bucket{}
		}
	}
	for i := range hk.heap {
		hk.heap[i] = FlowCount{}
	}
}

type minHeap []FlowCount

var _ heap.Interface = &minHeap{}

func (h minHeap) Len() int            { return len(h) }
func (h minHeap) Less(i, j int) bool  { return h[i].Count < h[j].Count }
func (h minHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *minHeap) Push(x interface{}) { *h = append(*h, x.(FlowCount)) }

func (h *minHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// Min returns the minimum count in the heap or 0 if the heap is empty.
func (h minHeap) Min() uint32 {
	return h[0].Count
}

// Find returns the index of the given flow in the heap so that it can be
// updated in-place (be sure to call heap.Fix() afterwards). It returns -1 if
// the flow doesn't exist in the heap.
func (h minHeap) Find(flow string) (i int) {
	for i := range h {
		if h[i].Flow == flow {
			return i
		}
	}
	return -1
}
