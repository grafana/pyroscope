package remotewrite

import (
	"context"
	"sync"

	"github.com/pyroscope-io/pyroscope/pkg/parser"
	"github.com/sirupsen/logrus"
)

type Putter interface {
	Put(ctx context.Context, put *parser.PutInput) error
}

// TODO(eh-am): move this to somehwere else?
type Paralellizer struct {
	log     *logrus.Logger
	putters []Putter
	wg      sync.WaitGroup
}

func NewParalellizer(log *logrus.Logger, putters ...Putter) *Paralellizer {
	return &Paralellizer{
		log:     log,
		putters: putters,
	}
}

func (p *Paralellizer) Put(ctx context.Context, pi *parser.PutInput) error {
	p.wg.Add(len(p.putters))

	// TODO(eh-am): add timeouts for each individual call
	for _, putter := range p.putters {
		// https://golang.org/doc/faq#closures_and_goroutines
		putter := putter
		// Clone the putInput since it will be read concurrently
		pi := pi.Clone()

		go func(pi *parser.PutInput) {
			defer p.wg.Done()
			err := putter.Put(ctx, pi)
			if err != nil {
				p.log.Error("Failed to parallelize put: ", err)
			}
		}(pi)
	}

	p.wg.Wait()

	//	if err := g.Wait(); err != nil {
	//		// swallow the error
	//		// TODO(eh-am): should we swallow errors?
	//		p.log.Error("Failed to parallelize put: ", err)
	//	}
	//
	return nil
}
