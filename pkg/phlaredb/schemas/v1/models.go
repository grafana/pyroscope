package v1

import googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"

type Models interface {
	*Profile | *InMemoryProfile |
		*googlev1.Location | *InMemoryLocation |
		*googlev1.Function | *InMemoryFunction |
		*googlev1.Mapping | *InMemoryMapping |
		*Stacktrace |
		string
}
