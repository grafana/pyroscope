package health

// Condition represents an aspect of pyroscope server health.
type Condition interface {
	Probe() (StatusMessage, error)
}

type StatusMessage struct {
	Status
	// The message is displayed to users.
	Message string
}

type Status int

const (
	NoData Status = iota
	Healthy
	Warning
	Critical
)

func (s Status) String() string {
	switch s {
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
