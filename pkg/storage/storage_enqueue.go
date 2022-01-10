package storage

import (
	"fmt"
	"runtime/debug"
)

func (s *Storage) Enqueue(input *PutInput) {
	select {
	case s.queue <- input:
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
			if err := s.safePut(input); err != nil {
				s.logger.WithField("key", input.Key).WithError(err).Error("error happened while ingesting data")
			}
		case <-s.stop:
			return
		}
	}
}

func (s *Storage) safePut(input *PutInput) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic recovered: %v; %v", r, string(debug.Stack()))
		}
	}()
	return s.Put(input)
}
