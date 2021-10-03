// pkg/server/health/controller.go

package health

import "strings"

// Controller performs probes of health conditions.
type Controller struct {
	C []*Condition
}

func (c Controller) Start() {
	for _, condition := range c.C {
		(*condition).MakeProbe()
	}

}
func (c Controller) Stop() {
	for _, condition := range c.C {
		(*condition).Stop()
	}
}

func (c Controller) Notification() []string {
	// This method should not access Conditions. Instead this message
	// should be built by controller after all the probes polled.
	var notification []string
	for _, condition := range c.C {
		switch (*condition).State() {
		case Critical:
		case Warning:
		default:
			notification = append(notification, (*condition).Message())
		}
	}
	return notification
}

func (c Controller) NotificationText() string {
	notification := c.Notification()
	str := strings.Join(notification, "\n")
	return str
}
