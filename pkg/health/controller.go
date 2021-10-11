package health

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type healthStatusHistory []HealthStatusMessage

// Controller performs probes of health conditions.
type Controller struct {
	conditions []Condition
	interval   time.Duration

	mutex         sync.Mutex
	ticker        *time.Ticker
	statusHistory map[Condition]healthStatusHistory
	logger        *logrus.Logger
}

func NewController(conditions []Condition, interval time.Duration, logger *logrus.Logger) *Controller {
	return &Controller{
		conditions: conditions,
		interval:   interval,
		logger:     logger,
	}
}

func (c *Controller) Start() {
	c.ticker = time.NewTicker(c.interval)
	c.statusHistory = make(map[Condition]healthStatusHistory)
	for _, condition := range c.conditions {
		c.statusHistory[condition] = make(healthStatusHistory, 5)
	}
	go c.run()
}

func (c *Controller) Stop() {
	c.ticker.Stop()
}

func (c *Controller) run() {
	for ; true; <-c.ticker.C {
		for _, condition := range c.conditions {
			c.mutex.Lock()

			history := c.statusHistory[condition]
			copy(history, history[1:])
			probe, err := condition.Probe()
			if err == nil {
				history[len(history)-1] = probe
			} else {
				errMessage := fmt.Sprintf("Error in Probing Condition %T", condition)
				c.logger.WithError(err).Error(errMessage)
			}

			c.mutex.Unlock()
		}
	}
}

func (c *Controller) aggregate() map[Condition]HealthStatusMessage {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	statusAggregate := make(map[Condition]HealthStatusMessage)
	for _, condition := range c.conditions {
		accumilator := HealthStatusMessage{NoData, ""}
		for _, status := range c.statusHistory[condition] {
			if status.HealthStatus > accumilator.HealthStatus {
				accumilator = status
			}
		}
		statusAggregate[condition] = accumilator
	}
	return statusAggregate
}

func (c *Controller) Notification() []string {
	messages := make([]string, 0)
	for _, status := range c.aggregate() {
		if status.HealthStatus > Healthy {
			messages = append(messages, status.Message)
		}
	}
	return messages
}

func (c *Controller) NotificationJSON() string {
	notification := c.Notification()
	msg, _ := json.Marshal(notification)
	return string(msg)
}
