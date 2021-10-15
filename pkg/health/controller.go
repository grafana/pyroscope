package health

import (
	"fmt"
	"strings"
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
				WithField("probe_name", fmt.Sprintf("%T", condition)).
				Warn("failed to make probe")
		}
		history[len(history)-1] = s
		var worst StatusMessage
		for _, x := range history {
			if x.Status > c.current[i].Status {
				worst = x
			}
		}
		c.current[i] = worst
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
func (c *Controller) NotificationText() string {
	var b strings.Builder
	for _, s := range c.Unhealthy() {
		b.WriteString(s.Message)
	}
	return b.String()
}
