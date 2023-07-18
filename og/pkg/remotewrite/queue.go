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

// this queue was based off storage/queue.go
// TODO(eh-am): merge with that one
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
	// Setup defaults
	if cfg.QueueWorkers == 0 {
		// This may be a very conservative value
		// Since it's IO bounded work
		cfg.QueueWorkers = numWorkers()
	}

	if cfg.QueueSize == 0 {
		cfg.QueueSize = 100
	}

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

func (q *IngestionQueue) initMetrics(queueSize int, queueWorkers int) {
	q.metrics.capacity.Add(float64(queueSize))
	q.metrics.numWorkers.Add(float64(queueWorkers))
}

func (q *IngestionQueue) Stop() {
	close(q.stop)
	q.wg.Wait()
}

func (q *IngestionQueue) Ingest(ctx context.Context, input *ingestion.IngestInput) error {
	select {
	case <-ctx.Done():
	case <-q.stop:
	case q.queue <- input:
		q.metrics.pendingItems.Inc()
		// Once input is queued, context cancellation is ignored.
		return nil
	default:
		// Drop data if the queue is full.
	}

	q.logger.WithField("key", input.Metadata.Key.Normalized()).Debugf("dropping since there's not enough space in the queue")
	q.metrics.droppedItems.Inc()
	return nil
}

func (q *IngestionQueue) runQueueWorker() {
	defer q.wg.Done()
	for {
		select {
		case input := <-q.queue:
			if err := q.safePut(input); err != nil {
				q.logger.WithField("key", input.Metadata.Key.Normalized()).WithError(err).Error("error happened while ingesting data")
			}
			q.metrics.pendingItems.Dec()
		case <-q.stop:
			return
		}
	}
}

func (q *IngestionQueue) safePut(input *ingestion.IngestInput) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic recovered: %v; %v", r, string(debug.Stack()))
		}
	}()
	// TODO(kolesnikovae): It's better to derive a context that is cancelled on Stop.
	return q.ingester.Ingest(context.TODO(), input)
}
