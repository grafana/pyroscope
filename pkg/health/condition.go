package health

// Condition represents an aspect of pyroscope server health.
type Condition interface {
	Probe() (HealthStatusMessage, error)
}
type HealthStatusMessage struct {
	HealthStatus HealthStatus
	Message      string
}

type HealthStatus int

const (
	NoData HealthStatus = iota
	Healthy
	Warning
	Critical
)

func (e HealthStatus) String() string {
	switch e {
	case Healthy:
		return "Healthy"
	case Warning:
		return "Warning"
	case Critical:
		return "Critical"
	default:
		return "Unknown"
	}
}
