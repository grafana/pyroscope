// pkg/server/health/controller.go

package health

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

var logger *logrus.Logger = logrus.New()

type HealthStatusHistory []HealthStatusMessage

// Controller performs probes of health conditions.
type ControllerBase interface {
	Start()
	Stop()
	Notification() []string
	NotificationJson() string
}
type Controller struct {
	Conditions []Condition
	Interval   time.Duration

	ticker        *time.Ticker
	statusHistory map[Condition]HealthStatusHistory
	mutex         sync.Mutex
}

func (c *Controller) Start() {
	c.ticker = time.NewTicker(c.Interval)
	c.statusHistory = make(map[Condition]HealthStatusHistory)
	for _, condition := range c.Conditions {
		c.statusHistory[condition] = make(HealthStatusHistory, 5)
	}
	go c.Sample()
}

func (c *Controller) Stop() {
	c.ticker.Stop()
}

func (c *Controller) Sample() {
	for ; true; <-c.ticker.C {
		for _, condition := range c.Conditions {

			c.mutex.Lock()

			history := c.statusHistory[condition]
			copy(history, history[1:])
			probe, err := condition.Probe()
			if err == nil {
				history[len(history)-1] = probe
			} else {
				errMessage := fmt.Sprintf("Error in Probing Condition %s", condition.GetName())
				logger.WithError(err).Error(errMessage)
			}

			c.mutex.Unlock()

		}
	}
}

func (c *Controller) aggregate() map[Condition]HealthStatusMessage {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	var statusAggregate map[Condition]HealthStatusMessage = make(map[Condition]HealthStatusMessage)
	for _, condition := range c.Conditions {
		var accumilator HealthStatusMessage = HealthStatusMessage{NoData, ""}
		for _, status := range c.statusHistory[condition] {
			if status.healthStatus > accumilator.healthStatus {
				accumilator = status
			}
		}
		statusAggregate[condition] = accumilator
	}
	return statusAggregate

}
func (c *Controller) Notification() []string {
	var messages []string = make([]string, 0)
	for _, status := range c.aggregate() {
		if status.healthStatus > Healthy {
			messages = append(messages, status.message)
		}
	}
	return messages
}

func (c *Controller) NotificationJson() string {
	notification := c.Notification()
	msg, _ := json.Marshal(notification)
	return string(msg)
}
