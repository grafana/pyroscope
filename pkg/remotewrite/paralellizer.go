package remotewrite

import (
	"context"

	"github.com/pyroscope-io/pyroscope/pkg/parser"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

type Putter interface {
	Put(ctx context.Context, put *parser.PutInput) error
}

// TODO(eh-am): move this to somehwere else?
type Paralellizer struct {
	log     *logrus.Logger
	putters []Putter
}

func NewParalellizer(log *logrus.Logger, putters ...Putter) *Paralellizer {
	return &Paralellizer{
		log:     log,
		putters: putters,
	}
}

func (p *Paralellizer) Put(ctx context.Context, pi *parser.PutInput) error {
	g, ctx := errgroup.WithContext(ctx)
	for _, putter := range p.putters {
		// https://golang.org/doc/faq#closures_and_goroutines
		putter := putter

		g.Go(func() error {
			// TODO(eh-am): not sure we want to straight up pass the same object
			// since pi
			return putter.Put(ctx, pi)
		})
	}

	if err := g.Wait(); err != nil {
		// swallow the error
		// TODO(eh-am): should we swallow errors?
		p.log.Error("Failed to parallelize put: ", err)
	}

	return nil
}
