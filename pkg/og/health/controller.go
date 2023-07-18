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
	current    []StatusMessage

	interval time.Duration
	logger   *logrus.Logger

	close chan struct{}
}

const historySize = 5

func NewController(logger *logrus.Logger, interval time.Duration, conditions ...Condition) *Controller {
	c := Controller{
		conditions: conditions,
		history:    make([][]StatusMessage, len(conditions)),
		current:    make([]StatusMessage, len(conditions)),
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
	c.probe()
	go func() {
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
	}()
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
				WithField("probe-name", fmt.Sprintf("%T", condition)).
				Warn("failed to make probe")
		}
		history[len(history)-1] = s
		current := s
		for _, x := range history {
			if x.Status > current.Status {
				current = x
			}
		}
		c.current[i] = current
	}
}

func (c *Controller) Unhealthy() []StatusMessage {
	c.m.RLock()
	defer c.m.RUnlock()
	m := make([]StatusMessage, 0, len(c.current))
	for _, x := range c.current {
		if x.Status > Healthy {
			m = append(m, x)
		}
	}
	return m
}

// NotificationText satisfies server.Notifier.
//
// TODO(kolesnikovae): I think we need to make UI notifications
//  structured (explicit status field) and support multiple messages.
//  At the moment there can be only one notification.
func (c *Controller) NotificationText() string {
	if u := c.Unhealthy(); len(u) > 0 {
		return u[0].Message
	}
	return ""
}

func (c *Controller) IsOutOfDiskSpace() bool {
	c.m.RLock()
	defer c.m.RUnlock()
	for i := range c.conditions {
		if _, ok := c.conditions[i].(DiskPressure); ok && c.current[i].Status > Healthy {
			return true
		}
	}
	return false
}
