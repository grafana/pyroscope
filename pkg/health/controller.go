package health

import (
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Controller performs probes of health conditions.
type Controller struct {
	m          sync.RWMutex
	conditions []Condition
	history    [][]StatusMessage
	current    []int

	interval time.Duration
	logger   *logrus.Logger

	close chan struct{}
}

const historySize = 5

func NewController(conditions []Condition, interval time.Duration, logger *logrus.Logger) *Controller {
	c := Controller{
		conditions: conditions,
		history:    make([][]StatusMessage, len(conditions)),
		current:    make([]int, len(conditions)),
		interval:   interval,
		logger:     logger,
		close:      make(chan struct{}),
	}
	for i := range c.history {
		c.history[i] = make([]StatusMessage, historySize)
	}
	return &c
}

func (c *Controller) Start() {
	t := time.NewTicker(c.interval)
	defer t.Stop()
	for {
		select {
		case <-c.close:
			return
		case <-t.C:
			c.probe()
		}
	}
}

func (c *Controller) Stop() { close(c.close) }

func (c *Controller) probe() {
	c.m.Lock()
	defer c.m.Unlock()
	for i, condition := range c.conditions {
		history := c.history[i]
		copy(history, history[1:])
		s, err := condition.Probe()
		if err != nil {
			s = StatusMessage{Message: err.Error()}
			c.logger.WithError(err).
				WithField("probe_name", fmt.Sprintf("%T", condition)).
				Warn("failed to make probe")
		}
		history[len(history)-1] = s
		for j, x := range history {
			if x.Status > history[c.current[i]].Status {
				c.current[i] = j
			}
		}
	}
}

func (c *Controller) Notification() []string {
	c.m.RLock()
	defer c.m.RUnlock()
	messages := make([]string, 0, len(c.conditions))
	for i, j := range c.current {
		s := c.history[i][j]
		if s.Status > Healthy {
			messages = append(messages, s.Message)
		}
	}
	return messages
}
