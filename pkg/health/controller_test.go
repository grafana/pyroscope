package health

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var dataMisc = []HealthStatusMessage{
	{Critical, "Critical"},
	{NoData, "NoData"},
	{Healthy, "Healthy"},
	{Warning, "Warning"},
}

var dataHealthy = []HealthStatusMessage{
	{Healthy, "Healthy"},
}

var dataWarning = []HealthStatusMessage{
	{Warning, "Warning"},
}

var dataCritical = []HealthStatusMessage{
	{Critical, "Critical"},
}

type MockCondition struct {
	MockData []HealthStatusMessage

	name  string
	index int
}

func (d *MockCondition) Probe() (HealthStatusMessage, error) {
	var status = d.MockData[d.index]
	status.Message = fmt.Sprintf("%s %s", status.Message, d.name)
	d.index = (d.index + 1) % len(d.MockData)
	return status, nil
}

func (d *MockCondition) GetName() string {
	return d.name
}

func (*MockCondition) Stop() {}

var _ = Describe("health", func() {
	testing.WithConfig(func(cfg **config.Config) {
		Describe("Controller", func() {
			It("Should support listening on multiple Conditions",
				func() {
					defer GinkgoRecover()
					condition1 := MockCondition{
						name:     "MockCondition1",
						MockData: dataHealthy,
					}
					condition2 := MockCondition{
						name:     "MockCondition2",
						MockData: dataCritical,
					}
					condition3 := MockCondition{
						name:     "MockCondition3",
						MockData: dataWarning,
					}

					healthController := NewController([]Condition{&condition1, &condition2, &condition3}, time.Millisecond, logrus.New())
					healthController.Start()
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
					condition := MockCondition{
						name:     "MockCondition",
						MockData: dataMisc,
					}

					healthController := NewController([]Condition{&condition}, time.Millisecond, logrus.New())
					healthController.Start()

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
					condition := MockCondition{
						name:     "MockCondition",
						MockData: dataHealthy,
					}

					healthController := NewController([]Condition{&condition}, time.Millisecond, logrus.New())
					healthController.Start()
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
					condition1 := MockCondition{
						name:     "MockCondition1",
						MockData: dataCritical,
					}
					condition2 := MockCondition{
						name:     "MockCondition2",
						MockData: dataWarning,
					}

					healthController := NewController([]Condition{&condition1, &condition2}, time.Millisecond, logrus.New())
					healthController.Start()
					time.Sleep(2 * time.Millisecond)

					jsonNotification := healthController.NotificationJSON()
					healthController.Stop()
					var arr []string
					_ = json.Unmarshal([]byte(jsonNotification), &arr)

					expectedNotification := []string{"Critical MockCondition1", "Warning MockCondition2"}
					Expect(arr).To(ContainElements(expectedNotification))
				},
			)
		})
	})
})
