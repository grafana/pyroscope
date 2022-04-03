package storage

import (
	"context"
	"fmt"
	"runtime/debug"
)

type putInputWithCtx struct {
	pi  *PutInput
	ctx context.Context
}

func (s *Storage) Enqueue(ctx context.Context, input *PutInput) {
	select {
	case s.queue <- &putInputWithCtx{input, ctx}:
	case <-s.stop:
	default:
		s.logger.WithField("key", input.Key).Error("storage queue is full, dropping a profile")
	}
}

func (s *Storage) startQueueWorkers() {
	s.queueWorkersWG.Add(s.queueWorkers)
	for i := 0; i < s.queueWorkers; i++ {
		go s.runQueueWorker()
	}
}

func (s *Storage) runQueueWorker() {
	defer s.queueWorkersWG.Done()
	for {
		select {
		case input := <-s.queue:
			if err := s.safePut(input.ctx, input.pi); err != nil {
				s.logger.WithField("key", input.pi.Key).WithError(err).Error("error happened while ingesting data")
			}
		case <-s.stop:
			return
		}
	}
}

func (s *Storage) safePut(ctx context.Context, input *PutInput) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic recovered: %v; %v", r, string(debug.Stack()))
		}
	}()
	// TODO(petethepig): not sure if retaining context is a good practice
	return s.Put(ctx, input)
}
