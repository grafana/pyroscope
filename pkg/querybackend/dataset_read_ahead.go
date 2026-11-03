package querybackend

import (
	"context"

	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/util"
)

type datasetReadAhead struct {
	ctx      context.Context
	cancel   context.CancelFunc
	sections []block.Section
	datasets []*block.Dataset
	opened   []*openedDataset
	depth    int
	next     int
	released int
}

type openedDataset struct {
	done chan struct{}
	err  error
}

func newDatasetReadAhead(
	ctx context.Context,
	datasets []*block.Dataset,
	sections []block.Section,
	depth int,
) *datasetReadAhead {
	r := &datasetReadAhead{
		sections: sections,
		datasets: datasets,
		opened:   make([]*openedDataset, len(datasets)),
		depth:    max(depth, 0),
	}
	r.ctx, r.cancel = context.WithCancel(ctx)
	return r
}

func (r *datasetReadAhead) acquire(i int) {
	for ; r.next < len(r.datasets) && r.next <= i+r.depth; r.next++ {
		ds, o := r.datasets[r.next], &openedDataset{done: make(chan struct{})}
		r.opened[r.next] = o
		go func() {
			defer close(o.done)
			o.err = util.RecoverPanic(func() error {
				return ds.Open(r.ctx, r.sections...)
			})()
		}()
	}
	<-r.opened[i].done
}

func (r *datasetReadAhead) release() {
	o := r.opened[r.released]
	<-o.done
	if o.err == nil {
		_ = r.datasets[r.released].Close()
	}
	r.released++
}

func (r *datasetReadAhead) close() {
	r.cancel()
	for r.released < r.next {
		r.release()
	}
}
