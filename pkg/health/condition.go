// pkg/server/health/health.go

package health

// Condition represents an aspect of pyroscope server health.
type Condition interface {
	GetName() string
	Probe() (HealthStatusMessage, error)
	Stop()
}
type HealthStatusMessage struct {
	healthStatus HealthStatus
	message      string
}

type HealthStatus int

const (
	NoData HealthStatus = iota
	Healthy
	Warning
	Critical
)

func (e HealthStatus) ToString() string {
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
