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
					condition1 := mockCondition{
						name:     "MockCondition1",
						mockData: dataHealthy,
					}
					condition2 := mockCondition{
						name:     "MockCondition2",
						mockData: dataCritical,
					}
					condition3 := mockCondition{
						name:     "MockCondition3",
						mockData: dataWarning,
					}

					healthController := NewController([]Condition{&condition1, &condition2, &condition3}, time.Millisecond, logrus.New())
					go healthController.Start()
					time.Sleep(2 * time.Millisecond)

					notification := healthController.Notification()
					healthController.Stop()

					expectedNotification := []string{"Critical MockCondition2", "Warning MockCondition3"}
					Expect(notification).To(ContainElements(expectedNotification))
				},
			)

			It("Should suppress 'flapping' on rapid status changes",
				func() {
					defer GinkgoRecover()
					condition := mockCondition{
						name:     "MockCondition",
						mockData: dataMisc,
					}

					healthController := NewController([]Condition{&condition}, time.Millisecond, logrus.New())
					go healthController.Start()

					var notifications []string
					for i := 0; i < 3; i++ {
						time.Sleep(2 * time.Millisecond)
						notifications = append(notifications, healthController.Notification()[0])
					}
					healthController.Stop()

					expectedNotification := "Critical MockCondition"
					Expect(notifications).To(Equal([]string{expectedNotification, expectedNotification, expectedNotification}))
				},
			)

			It("Should return empty notification if status healthy",
				func() {
					defer GinkgoRecover()
					condition := mockCondition{
						name:     "MockCondition",
						mockData: dataHealthy,
					}

					healthController := NewController([]Condition{&condition}, time.Millisecond, logrus.New())
					go healthController.Start()
					time.Sleep(2 * time.Millisecond)

					notification := healthController.Notification()
					healthController.Stop()

					emptyNotification := make([]string, 0)
					Expect(notification).To(Equal(emptyNotification))
				},
			)

			It("Should format notification as JSON",
				func() {
					defer GinkgoRecover()
					condition1 := mockCondition{
						name:     "MockCondition1",
						mockData: dataCritical,
					}
					condition2 := mockCondition{
						name:     "MockCondition2",
						mockData: dataWarning,
					}

					healthController := NewController([]Condition{&condition1, &condition2}, time.Millisecond, logrus.New())
					go healthController.Start()
					time.Sleep(2 * time.Millisecond)

					actualNotification := healthController.Notification()
					healthController.Stop()

					expectedNotification := []string{"Critical MockCondition1", "Warning MockCondition2"}
					Expect(actualNotification).To(ConsistOf(expectedNotification))
				},
			)
		})
	})
})
