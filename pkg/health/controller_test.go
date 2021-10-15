package health

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
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
	dataMisc := []StatusMessage{
		{Critical, "Critical"},
		{NoData, "NoData"},
		{Healthy, "Healthy"},
		{Warning, "Warning"},
	}

	dataHealthy := []StatusMessage{{Healthy, "Healthy"}}
	dataWarning := []StatusMessage{{Warning, "Warning"}}
	dataCritical := []StatusMessage{{Critical, "Critical"}}

	testing.WithConfig(func(cfg **config.Config) {
		Describe("Controller", func() {
			It("Should support listening on multiple Conditions",
				func() {
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
				},
			)

			It("Should suppress 'flapping' on rapid status changes",
				func() {
					defer GinkgoRecover()
					condition := &mockCondition{name: "MockCondition", mockData: dataMisc}

					healthController := NewController(logrus.New(), time.Millisecond, condition)
					healthController.Start()

					var notifications []StatusMessage
					for i := 0; i < 3; i++ {
						notifications = append(notifications, healthController.Unhealthy()[0])
					}
					healthController.Stop()

					expectedNotification := StatusMessage{Critical, "Critical MockCondition"}
					Expect(notifications).To(Equal([]StatusMessage{
						expectedNotification,
						expectedNotification,
						expectedNotification,
					}))
				},
			)

			It("Should return empty notification if status healthy",
				func() {
					defer GinkgoRecover()
					condition := &mockCondition{name: "MockCondition", mockData: dataHealthy}

					healthController := NewController(logrus.New(), time.Millisecond, condition)
					healthController.Start()

					notification := healthController.Unhealthy()
					healthController.Stop()

					Expect(notification).To(BeEmpty())
				},
			)

			It("Should format notification as JSON",
				func() {
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
				},
			)
		})
	})
})
