// pkg/server/health/health.go

package health

// Condition represents an aspect of pyroscope server health.
type Condition interface {
	State() State
	Message() string
	Stop() error
	MakeProbe() error
}

type State int

const (
	NoData State = iota
	Healthy
	Warning
	Critical
)
