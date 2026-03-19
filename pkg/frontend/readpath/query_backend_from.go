package readpath

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/grafana/dskit/flagext"
	"gopkg.in/yaml.v3"
)

// QueryBackendFrom is a flag/config type that accepts either an RFC3339
// timestamp or the special value "auto". When set to "auto", the split
// point is resolved at query time by looking up the tenant's oldest
// profile time in the metastore.
type QueryBackendFrom struct {
	Auto bool
	Time time.Time
}

func (q QueryBackendFrom) IsZero() bool {
	return !q.Auto && q.Time.IsZero()
}

// Set implements flag.Value.
func (q *QueryBackendFrom) Set(s string) error {
	if s == "auto" {
		q.Auto = true
		q.Time = time.Time{}
		return nil
	}
	q.Auto = false
	return (*flagext.Time)(&q.Time).Set(s)
}

// String implements flag.Value.
func (q QueryBackendFrom) String() string {
	if q.Auto {
		return "auto"
	}
	return flagext.Time(q.Time).String()
}

func (q QueryBackendFrom) MarshalJSON() ([]byte, error) {
	return json.Marshal(q.String())
}

func (q *QueryBackendFrom) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	return q.Set(s)
}

func (q QueryBackendFrom) MarshalYAML() (interface{}, error) {
	return q.String(), nil
}

func (q *QueryBackendFrom) UnmarshalYAML(value *yaml.Node) error {
	if value.Value == "" {
		return nil
	}
	return q.Set(value.Value)
}

func (q QueryBackendFrom) MarshalText() ([]byte, error) {
	return []byte(q.String()), nil
}

func (q *QueryBackendFrom) UnmarshalText(data []byte) error {
	s := string(data)
	if s == "" {
		return nil
	}
	return q.Set(s)
}

// SplitTime returns the split timestamp for routing queries.
// For a fixed timestamp, it returns the time directly.
// For "auto" mode, it queries the metastore for the tenant's oldest profile time.
// Returns zero time if the split cannot be determined.
func (q QueryBackendFrom) SplitTime(tenantOldestProfileTime func() (time.Time, error)) (time.Time, error) {
	if !q.Auto {
		return q.Time, nil
	}
	t, err := tenantOldestProfileTime()
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to resolve auto split time: %w", err)
	}
	return t, nil
}
