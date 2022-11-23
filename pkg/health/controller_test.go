package health

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

type mockCondition struct {
	mockData []StatusMessage
	name     string
	index    int
}

func (d *mockCondition) Probe() (StatusMessage, error) {
	var status = d.mockData[d.index]
	status.Message = fmt.Sprintf("%s %s", status.Message, d.name)
	d.index = (d.index + 1) % len(d.mockData)
	return status, nil
}

var _ = Describe("health", func() {
	dataHealthy := []StatusMessage{{Healthy, "Healthy"}}
	dataWarning := []StatusMessage{{Warning, "Warning"}}
	dataCritical := []StatusMessage{{Critical, "Critical"}}

	testing.WithConfig(func(cfg **config.Config) {
		Describe("Controller", func() {
			It("Should support listening on multiple Conditions", func() {
				defer GinkgoRecover()

				condition1 := &mockCondition{name: "MockCondition1", mockData: dataHealthy}
				condition2 := &mockCondition{name: "MockCondition2", mockData: dataCritical}
				condition3 := &mockCondition{name: "MockCondition3", mockData: dataWarning}

				healthController := NewController(logrus.New(), time.Millisecond, condition1, condition2, condition3)
				healthController.Start()

				notification := healthController.Unhealthy()
				healthController.Stop()

				Expect(notification).To(ContainElements([]StatusMessage{
					{Critical, "Critical MockCondition2"},
					{Warning, "Warning MockCondition3"},
				}))
			})

			It("Should suppress 'flapping' on rapid status changes", func() {
				defer GinkgoRecover()

				condition := &mockCondition{mockData: []StatusMessage{
					{Status: Healthy},
					{Status: Healthy},
					{Status: Warning},
					{Status: Healthy},
					{Status: Critical},
					{Status: Healthy},
					{Status: Critical},
					{Status: Healthy},
					{Status: Healthy},
					{Status: Healthy},
					{Status: Healthy},
				}}

				healthController := NewController(logrus.New(), time.Minute, condition)
				healthController.Start()
				Expect(healthController.Unhealthy()).To(BeEmpty())
				healthController.probe()
				Expect(healthController.Unhealthy()).To(BeEmpty())

				healthController.probe()
				requireStatus(healthController.Unhealthy(), Warning)
				healthController.probe()
				requireStatus(healthController.Unhealthy(), Warning)
				healthController.probe()
				requireStatus(healthController.Unhealthy(), Critical)
				healthController.probe()
				requireStatus(healthController.Unhealthy(), Critical)
				healthController.probe()
				requireStatus(healthController.Unhealthy(), Critical)
				healthController.probe()
				healthController.probe()
				healthController.probe()
				healthController.probe()
				healthController.probe()

				Expect(healthController.Unhealthy()).To(BeEmpty())
				healthController.Stop()
			})

			It("Should return empty notification if status healthy", func() {
				defer GinkgoRecover()

				condition := &mockCondition{name: "MockCondition", mockData: dataHealthy}

				healthController := NewController(logrus.New(), time.Millisecond, condition)
				healthController.Start()

				notification := healthController.Unhealthy()
				healthController.Stop()

				Expect(notification).To(BeEmpty())
			})

			It("Satisfies notifier interface", func() {
				defer GinkgoRecover()

				condition1 := &mockCondition{name: "MockCondition1", mockData: dataCritical}
				condition2 := &mockCondition{name: "MockCondition2", mockData: dataWarning}

				healthController := NewController(logrus.New(), time.Millisecond, condition1, condition2)
				healthController.Start()

				actualNotification := healthController.Unhealthy()
				healthController.Stop()

				Expect(actualNotification).To(ConsistOf([]StatusMessage{
					{Critical, "Critical MockCondition1"},
					{Warning, "Warning MockCondition2"},
				}))
			})
		})
	})
})

func requireStatus(s []StatusMessage, x Status) {
	Expect(len(s)).To(Equal(1))
	Expect(s[0].Status).To(Equal(x))
}
