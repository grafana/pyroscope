package profiledump

import (
	"time"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

const schemaVersion = 1

// Metadata describes a capture in its JSON sidecar.
type Metadata struct {
	SchemaVersion     uint16               `json:"schema_version"`
	CapturedAt        time.Time            `json:"captured_at"`
	TenantID          string               `json:"tenant_id"`
	SourceProtocol    string               `json:"source_protocol"`
	Format            string               `json:"format"`
	Labels            []*typesv1.LabelPair `json:"labels,omitempty"`
	OriginalProfileID string               `json:"original_profile_id,omitempty"`
	DistributorID     string               `json:"distributor_id"`
	CaptureID         string               `json:"capture_id"`
	PayloadSize       int64                `json:"payload_size"`
}
