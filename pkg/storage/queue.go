package storage

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
)

type IngestionQueue struct {
	logger logrus.FieldLogger
	putter Putter

	wg    sync.WaitGroup
	queue chan *PutInput
	stop  chan struct{}

	discardedTotal prometheus.Counter
}

const (
	defaultQueueSize = 100
	defaultWorkers   = 1
)

func NewIngestionQueue(logger logrus.FieldLogger, putter Putter, r prometheus.Registerer, c *Config) *IngestionQueue {
	queueSize := c.queueSize
	if queueSize == 0 {
		queueSize = defaultQueueSize
	}
	queueWorkers := c.queueWorkers
	if queueWorkers == 0 {
		queueWorkers = defaultWorkers
	}

	q := IngestionQueue{
		logger: logger,
		putter: putter,
		queue:  make(chan *PutInput, queueSize),
		stop:   make(chan struct{}),

		discardedTotal: promauto.With(r).NewCounter(prometheus.CounterOpts{
			Name: "pyroscope_ingestion_queue_discarded_total",
			Help: "number of ingestion requests discarded",
		}),
	}

	q.wg.Add(queueWorkers)
	for i := 0; i < queueWorkers; i++ {
		go q.runQueueWorker()
	}

	return &q
}

func (s *IngestionQueue) Stop() {
	close(s.stop)
	s.wg.Wait()
}

func (s *IngestionQueue) Put(ctx context.Context, input *PutInput) error {
	select {
	case <-ctx.Done():
	case <-s.stop:
	case s.queue <- input:
		// Once input is queued, context cancellation is ignored.
		return nil
	default:
		// Drop data if the queue is full.
	}
	s.discardedTotal.Inc()
	return nil
}

func (s *IngestionQueue) runQueueWorker() {
	defer s.wg.Done()
	for {
		select {
		case input, ok := <-s.queue:
			if ok {
				if err := s.safePut(input); err != nil {
					s.logger.WithField("key", input.Key.Normalized()).WithError(err).Error("error happened while ingesting data")
				}
			}
		case <-s.stop:
			return
		}
	}
}

func (s *IngestionQueue) safePut(input *PutInput) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic recovered: %v; %v", r, string(debug.Stack()))
		}
	}()
	// TODO(kolesnikovae): It's better to derive a context that is cancelled on Stop.
	return s.putter.Put(context.TODO(), input)
}
