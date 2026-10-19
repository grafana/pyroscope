package querybackend

import (
	"sync"

	"github.com/grafana/pyroscope/v2/pkg/model"
)

// maxPartialTrees limits the number of trees an aggregator merges into
// concurrently: each holds up to a full copy of the result.
const maxPartialTrees = 4

// treeMergePool lets concurrent callers merge into separate partial trees,
// which are combined once all the inputs are merged. Merging is associative,
// and truncation only applies to the combined tree.
type treeMergePool struct {
	mu   sync.Mutex
	cond *sync.Cond
	free []*model.FlatTree
	all  int
}

func newTreeMergePool() *treeMergePool {
	p := new(treeMergePool)
	p.cond = sync.NewCond(&p.mu)
	return p
}

func (p *treeMergePool) merge(fn func(*model.FlatTree) error) error {
	p.mu.Lock()
	for len(p.free) == 0 && p.all >= maxPartialTrees {
		p.cond.Wait()
	}
	var t *model.FlatTree
	if n := len(p.free); n > 0 {
		t, p.free = p.free[n-1], p.free[:n-1]
	} else {
		t = model.NewFlatTree()
		p.all++
	}
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.free = append(p.free, t)
		p.mu.Unlock()
		p.cond.Signal()
	}()
	return fn(t)
}

// tree combines the partial trees. It must not be called concurrently
// with merge.
func (p *treeMergePool) tree() *model.FlatTree {
	trees := p.free
	p.free, p.all = nil, 0
	if len(trees) == 0 {
		return model.NewFlatTree()
	}
	for len(trees) > 1 {
		half := len(trees) / 2
		var wg sync.WaitGroup
		for i := 0; i < half; i++ {
			wg.Add(1)
			go func(dst, src *model.FlatTree) {
				defer wg.Done()
				dst.MergeTree(src)
			}(trees[i], trees[len(trees)-1-i])
		}
		wg.Wait()
		trees = trees[:len(trees)-half]
	}
	return trees[0]
}
