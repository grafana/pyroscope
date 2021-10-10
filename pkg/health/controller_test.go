package health

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var dataMisc []HealthStatusMessage = []HealthStatusMessage{
	{Critical, "Critical"},
	{NoData, "NoData"},
	{Healthy, "Healthy"},
	{Warning, "Warning"},
}

var dataHealthy []HealthStatusMessage = []HealthStatusMessage{
	{Healthy, "Healthy"},
}

var dataWarning []HealthStatusMessage = []HealthStatusMessage{
	{Warning, "Warning"},
}

var dataCritical []HealthStatusMessage = []HealthStatusMessage{
	{Critical, "Critical"},
}

type MockCondition struct {
	MockData []HealthStatusMessage

	name  string
	index int
}

func (d *MockCondition) Probe() (HealthStatusMessage, error) {
	var status HealthStatusMessage = d.MockData[d.index]
	status.message = fmt.Sprintf("%s %s", status.message, d.name)
	d.index = (d.index + 1) % len(d.MockData)
	println(d.index)
	return status, nil
}

func (d *MockCondition) GetName() string {
	return d.name
}

func (d *MockCondition) Stop() {}

var _ = Describe("health", func() {
	testing.WithConfig(func(cfg **config.Config) {
		Describe("Controller", func() {
			It("Should be critical",
				func() {
					defer GinkgoRecover()
					var condition = MockCondition{
						name:     "MockCondition",
						MockData: dataMisc,
					}
					var healthController = Controller{
						Interval:   time.Millisecond,
						Conditions: []Condition{&condition},
					}
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
			It("Should be healthy",
				func() {
					defer GinkgoRecover()
					var condition = MockCondition{
						name:     "MockCondition",
						MockData: dataHealthy,
					}
					var healthController = Controller{
						Interval:   time.Millisecond,
						Conditions: []Condition{&condition},
					}
					healthController.Start()
					time.Sleep(2 * time.Millisecond)
					notification := healthController.Notification()
					healthController.Stop()
					emptyNotification := make([]string, 0)
					Expect(notification).To(Equal(emptyNotification))
				},
			)
			It("Should be healthy",
				func() {
					defer GinkgoRecover()
					var condition1 = MockCondition{
						name:     "MockCondition1",
						MockData: dataHealthy,
					}
					var condition2 = MockCondition{
						name:     "MockCondition2",
						MockData: dataCritical,
					}
					var condition3 = MockCondition{
						name:     "MockCondition3",
						MockData: dataWarning,
					}

					var healthController = Controller{
						Interval:   time.Millisecond,
						Conditions: []Condition{&condition1, &condition2, &condition3},
					}
					healthController.Start()
					time.Sleep(2 * time.Millisecond)
					notification := healthController.Notification()
					healthController.Stop()
					expectedNotification := []string{"Critical MockCondition2", "Warning MockCondition3"}
					Expect(notification).To(ContainElements(expectedNotification))
				},
			)

			It("Should",
				func() {
					var condition1 = MockCondition{
						name:     "MockCondition1",
						MockData: dataCritical,
					}
					var condition2 = MockCondition{
						name:     "MockCondition2",
						MockData: dataWarning,
					}

					var healthController = Controller{
						Interval:   time.Millisecond,
						Conditions: []Condition{&condition1, &condition2},
					}
					healthController.Start()
					time.Sleep(2 * time.Millisecond)
					jsonNotification := healthController.NotificationJson()
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
