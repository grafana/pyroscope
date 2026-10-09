package profiledump

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/grafana/dskit/tenant"
	"github.com/oklog/ulid/v2"
)

const NativeObjectPrefix = ObjectPrefix + "native/"

const nativePartitionLayout = "2006-01-02/15/04"

// NativeObjectKey contains the identity and both sibling names of a validated key.
// CaptureTime is the ULID time in UTC milliseconds.
type NativeObjectKey struct {
	TenantID    string
	CaptureID   ulid.ULID
	CaptureTime time.Time
	PayloadKey  string
	MetadataKey string
}

// ValidateNativeTenant uses the shared tenant path policy, requiring a nonempty ID.
func ValidateNativeTenant(id string) error {
	if id == "" {
		return fmt.Errorf("tenant_id is required")
	}
	return tenant.ValidTenantID(id)
}

// NewNativeObjectKey returns a payload key and random ULID from one server capture time.
func NewNativeObjectKey(tenantID string, capturedAt time.Time) (string, ulid.ULID, error) {
	if err := ValidateNativeTenant(tenantID); err != nil {
		return "", ulid.ULID{}, err
	}
	if err := validateCaptureTime(capturedAt); err != nil {
		return "", ulid.ULID{}, err
	}
	id, err := ulid.New(ulid.Timestamp(capturedAt), rand.Reader)
	if err != nil {
		return "", ulid.ULID{}, fmt.Errorf("generate capture ID: %w", err)
	}
	key := NativeObjectPrefix + tenantID + "/" + capturedAt.UTC().Format(nativePartitionLayout) + "/" + id.String() + ".pprof"
	return key, id, nil
}

// ParseNativeObjectKey validates a complete payload or sidecar key before deriving
// sibling names. It never cleans, unescapes, or normalizes an input path.
func ParseNativeObjectKey(key string) (NativeObjectKey, error) {
	const maxKeyBytes = len(NativeObjectPrefix) + tenant.MaxTenantIDLength + 1 + len(nativePartitionLayout) + 1 + 26 + len(".pprof")
	if len(key) > maxKeyBytes || !strings.HasPrefix(key, NativeObjectPrefix) {
		return NativeObjectKey{}, fmt.Errorf("invalid native capture key prefix or length")
	}
	parts := strings.Split(strings.TrimPrefix(key, NativeObjectPrefix), "/")
	if len(parts) != 5 {
		return NativeObjectKey{}, fmt.Errorf("invalid native capture key shape")
	}
	if err := ValidateNativeTenant(parts[0]); err != nil {
		return NativeObjectKey{}, err
	}
	partition := strings.Join(parts[1:4], "/")
	minute, err := time.Parse(nativePartitionLayout, partition)
	if err != nil || minute.Format(nativePartitionLayout) != partition {
		return NativeObjectKey{}, fmt.Errorf("invalid native capture minute partition")
	}
	file := parts[4]
	if !((len(file) == 26+len(".pprof") && strings.HasSuffix(file, ".pprof")) ||
		(len(file) == 26+len(".json") && strings.HasSuffix(file, ".json"))) {
		return NativeObjectKey{}, fmt.Errorf("invalid native capture filename")
	}
	id, err := parseCaptureID(file[:26])
	if err != nil {
		return NativeObjectKey{}, err
	}
	capturedAt := ulid.Time(id.Time()).UTC()
	if err := validateCaptureTime(capturedAt); err != nil {
		return NativeObjectKey{}, err
	}
	if capturedAt.Format(nativePartitionLayout) != partition {
		return NativeObjectKey{}, fmt.Errorf("capture minute partition does not match ULID")
	}
	stem := NativeObjectPrefix + parts[0] + "/" + partition + "/" + id.String()
	return NativeObjectKey{
		TenantID: parts[0], CaptureID: id, CaptureTime: capturedAt,
		PayloadKey: stem + ".pprof", MetadataKey: stem + ".json",
	}, nil
}
