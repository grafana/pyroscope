// this queue was based off storage/queue.go
package remotewrite

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
	"github.com/sirupsen/logrus"
)

type IngestionQueue struct {
	logger   logrus.FieldLogger
	ingester ingestion.Ingester

	wg    sync.WaitGroup
	queue chan *ingestion.IngestInput
	stop  chan struct{}

	metrics *queueMetrics
}

// NewIngestionQueue creates an IngestionQueue
// Notice how a config.RemoteWriteTarget is taken as argument, even though
// not all fields are used. This is done to simplify the API, as the alternative
// is to take multiple arguments
func NewIngestionQueue(logger logrus.FieldLogger, reg prometheus.Registerer, ingester ingestion.Ingester, targetName string, cfg config.RemoteWriteTarget) *IngestionQueue {
	q := IngestionQueue{
		logger:   logger,
		ingester: ingester,
		queue:    make(chan *ingestion.IngestInput, cfg.QueueSize),
		stop:     make(chan struct{}),
		metrics:  newQueueMetrics(reg, targetName, cfg.Address),
	}

	q.wg.Add(cfg.QueueWorkers)
	for i := 0; i < cfg.QueueWorkers; i++ {
		go q.runQueueWorker()
	}

	q.metrics.mustRegister()
	q.initMetrics(cfg.QueueSize, cfg.QueueWorkers)

	return &q
}

func (iq *IngestionQueue) initMetrics(queueSize int, queueWorkers int) {
	iq.metrics.capacity.Add(float64(queueSize))
	iq.metrics.numWorkers.Add(float64(queueWorkers))
}

func (s *IngestionQueue) Stop() {
	close(s.stop)
	s.wg.Wait()
}

func (s *IngestionQueue) Ingest(ctx context.Context, input *ingestion.IngestInput) error {
	select {
	case <-ctx.Done():
	case <-s.stop:
	case s.queue <- input:
		// TODO(eh-am): bump metric
		// Once input is queued, context cancellation is ignored.
		return nil
	default:
		// Drop data if the queue is full.
	}
	//	s.discardedTotal.Inc()
	return nil
}

func (s *IngestionQueue) runQueueWorker() {
	defer s.wg.Done()
	for {
		select {
		case input, ok := <-s.queue:
			if ok {
				// TODO(eh-am): decrease metric
				if err := s.safePut(input); err != nil {
					s.logger.WithField("key", input.Metadata.Key.Normalized()).WithError(err).Error("error happened while ingesting data")
				}
			}
		case <-s.stop:
			return
		}
	}
}

func (s *IngestionQueue) safePut(input *ingestion.IngestInput) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic recovered: %v; %v", r, string(debug.Stack()))
		}
	}()
	// TODO(kolesnikovae): It's better to derive a context that is cancelled on Stop.
	return s.ingester.Ingest(context.TODO(), input)
}
