package remotewrite

import (
	"context"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
)

// TODO(eh-am): move this to somehwere else?
type Parallelizer struct {
	log       *logrus.Logger
	ingesters []ingestion.Ingester
	wg        sync.WaitGroup
}

func NewParallelizer(log *logrus.Logger, ingesters ...ingestion.Ingester) *Parallelizer {
	return &Parallelizer{
		log:       log,
		ingesters: ingesters,
	}
}

func (p *Parallelizer) Ingest(ctx context.Context, in *ingestion.IngestInput) error {
	p.wg.Add(len(p.ingesters))

	// TODO(eh-am): add timeouts for each individual call
	for _, putter := range p.ingesters {
		// https://golang.org/doc/faq#closures_and_goroutines
		putter := putter
		go func(in *ingestion.IngestInput) {
			defer p.wg.Done()
			err := putter.Ingest(ctx, in)
			if err != nil {
				p.log.Error("Failed to parallelize put: ", err)
			}
		}(in)
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
