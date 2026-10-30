package profiledump

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/grafana/dskit/tenant"
	"github.com/oklog/ulid/v2"

	"github.com/grafana/pyroscope/v2/pkg/phlaredb/bucket"
)

// ObjectPrefix holds captures beneath the configured storage prefix.
// Keys are <tenant>/YYYY-MM-DD/HH/MM/<capture-id>.pprof with a sibling .json sidecar.
// The payload preserves original bytes, even when empty or malformed.
const ObjectPrefix = bucket.PyroscopeInternalsPrefix + "/profile-dumps/"

const partitionLayout = "2006-01-02/15/04"

// ObjectKey contains the identity and both sibling names of a validated key.
// CaptureTime is the ULID time in UTC milliseconds.
type ObjectKey struct {
	TenantID    string
	CaptureID   ulid.ULID
	CaptureTime time.Time
	PayloadKey  string
	MetadataKey string
}

// ValidateTenant uses the shared tenant path policy, requiring a nonempty ID.
func ValidateTenant(id string) error {
	if id == "" {
		return fmt.Errorf("tenant_id is required")
	}
	return tenant.ValidTenantID(id)
}

// NewObjectKey returns a payload key and random ULID from one server capture time.
func NewObjectKey(tenantID string, capturedAt time.Time) (string, ulid.ULID, error) {
	if err := ValidateTenant(tenantID); err != nil {
		return "", ulid.ULID{}, err
	}
	id, err := ulid.New(ulid.Timestamp(capturedAt), rand.Reader)
	if err != nil {
		return "", ulid.ULID{}, fmt.Errorf("generate capture ID: %w", err)
	}
	key := ObjectPrefix + tenantID + "/" + capturedAt.UTC().Format(partitionLayout) + "/" + id.String() + ".pprof"
	return key, id, nil
}

// ParseObjectKey validates a complete payload or sidecar key before deriving sibling names.
func ParseObjectKey(key string) (ObjectKey, error) {
	const maxKeyBytes = len(ObjectPrefix) + tenant.MaxTenantIDLength + len("/") + len(partitionLayout) + len("/") + ulid.EncodedSize + len(".pprof")
	if len(key) > maxKeyBytes || !strings.HasPrefix(key, ObjectPrefix) {
		return ObjectKey{}, fmt.Errorf("invalid capture key prefix or length")
	}
	// Layout after the prefix: <tenant>/YYYY-MM-DD/HH/MM/<ULID>.<pprof|json>.
	parts := strings.Split(strings.TrimPrefix(key, ObjectPrefix), "/")
	if len(parts) != 5 {
		return ObjectKey{}, fmt.Errorf("invalid capture key shape")
	}
	if err := ValidateTenant(parts[0]); err != nil {
		return ObjectKey{}, err
	}
	partition := strings.Join(parts[1:4], "/")
	minute, err := time.Parse(partitionLayout, partition)
	if err != nil || minute.Format(partitionLayout) != partition {
		return ObjectKey{}, fmt.Errorf("invalid capture minute partition")
	}
	file := parts[4]
	if !((len(file) == ulid.EncodedSize+len(".pprof") && strings.HasSuffix(file, ".pprof")) ||
		(len(file) == ulid.EncodedSize+len(".json") && strings.HasSuffix(file, ".json"))) {
		return ObjectKey{}, fmt.Errorf("invalid capture filename")
	}
	id, err := parseCaptureID(file[:ulid.EncodedSize])
	if err != nil {
		return ObjectKey{}, err
	}
	capturedAt := ulid.Time(id.Time()).UTC()
	if capturedAt.Format(partitionLayout) != partition {
		return ObjectKey{}, fmt.Errorf("capture minute partition does not match ULID")
	}
	stem := ObjectPrefix + parts[0] + "/" + partition + "/" + id.String()
	return ObjectKey{
		TenantID: parts[0], CaptureID: id, CaptureTime: capturedAt,
		PayloadKey: stem + ".pprof", MetadataKey: stem + ".json",
	}, nil
}

func parseCaptureID(s string) (ulid.ULID, error) {
	id, err := ulid.ParseStrict(s)
	if err != nil {
		return ulid.ULID{}, fmt.Errorf("invalid capture ID: %w", err)
	}
	if id.String() != s {
		return ulid.ULID{}, fmt.Errorf("noncanonical capture ID")
	}
	return id, nil
}
