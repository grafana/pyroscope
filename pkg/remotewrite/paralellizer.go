package remotewrite

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
)

// TODO(eh-am): move this to somehwere else?
type Parallelizer struct {
	log       *logrus.Logger
	ingesters []ingestion.Ingester
}

func NewParallelizer(log *logrus.Logger, ingesters ...ingestion.Ingester) *Parallelizer {
	return &Parallelizer{
		log:       log,
		ingesters: ingesters,
	}
}

func (p *Parallelizer) Ingest(ctx context.Context, in *ingestion.IngestInput) error {
	var wg sync.WaitGroup
	wg.Add(len(p.ingesters))

	// TODO(eh-am): add timeouts for each individual call
	for _, putter := range p.ingesters {
		// https://golang.org/doc/faq#closures_and_goroutines
		putter := putter
		go func(in *ingestion.IngestInput) {
			defer wg.Done()
			err := p.safeIngest(ctx, in, putter)
			if err != nil {
				p.log.Error("Failed to parallelize put: ", err)
			}
		}(in)
	}

	wg.Wait()

	return nil
}

// This is required since ingester.Ingest may panic
func (p *Parallelizer) safeIngest(ctx context.Context, input *ingestion.IngestInput, ingester ingestion.Ingester) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic recovered: %v; %v", r, string(debug.Stack()))
		}
	}()
	return ingester.Ingest(ctx, input)
}
