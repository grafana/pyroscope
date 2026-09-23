package profiledump

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

const ObjectPrefix = "profile-debug-dumps/"

// Format identifies the native format, independently of compression or framing.
type Format string

const (
	FormatPprof      Format = "pprof"
	FormatJFR        Format = "jfr"
	FormatOTLP       Format = "otlp"
	FormatTrie       Format = "trie"
	FormatTree       Format = "tree"
	FormatLines      Format = "lines"
	FormatGroups     Format = "groups"
	FormatSpeedscope Format = "speedscope"
)

func (f Format) valid() bool {
	switch f {
	case FormatPprof, FormatJFR, FormatOTLP, FormatTrie, FormatTree, FormatLines, FormatGroups, FormatSpeedscope:
		return true
	default:
		return false
	}
}

// ObjectKey holds validated key fields. CaptureTime is the ULID time in UTC milliseconds.
type ObjectKey struct {
	TenantID    string
	CaptureID   ulid.ULID
	CaptureTime time.Time
	Format      Format
}

// EncodeTenant returns one canonical, unpadded base64url path segment.
func EncodeTenant(tenant string) (string, error) {
	if err := validateText("tenant_id", tenant, MaxTenantBytes, true); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString([]byte(tenant)), nil
}

// DecodeTenant rejects noncanonical encodings rather than normalizing them.
func DecodeTenant(segment string) (string, error) {
	if len(segment) == 0 || len(segment) > base64.RawURLEncoding.EncodedLen(MaxTenantBytes) {
		return "", fmt.Errorf("invalid tenant segment length")
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(segment)
	if err != nil {
		return "", fmt.Errorf("invalid tenant encoding: %w", err)
	}
	tenant := string(b)
	canonical, err := EncodeTenant(tenant)
	if err != nil {
		return "", err
	}
	if canonical != segment {
		return "", fmt.Errorf("noncanonical tenant encoding")
	}
	return tenant, nil
}

// NewObjectKey uses server capture time and a random ULID. Use the ID in Metadata.
// Retries are not deduplicated.
func NewObjectKey(tenant string, capturedAt time.Time, format Format) (string, ulid.ULID, error) {
	return newObjectKey(tenant, capturedAt, format, rand.Reader)
}

func newObjectKey(tenant string, capturedAt time.Time, format Format, entropy io.Reader) (string, ulid.ULID, error) {
	segment, err := EncodeTenant(tenant)
	if err != nil {
		return "", ulid.ULID{}, err
	}
	if !format.valid() {
		return "", ulid.ULID{}, fmt.Errorf("invalid native format")
	}
	if err := validateCaptureTime(capturedAt); err != nil {
		return "", ulid.ULID{}, err
	}
	id, err := ulid.New(ulid.Timestamp(capturedAt), entropy)
	if err != nil {
		return "", ulid.ULID{}, fmt.Errorf("generate capture ID: %w", err)
	}
	key := ObjectPrefix + segment + "/" + capturedAt.UTC().Format("2006/01/02") + "/" + id.String() + "-" + string(format) + ".pyrdump"
	return key, id, nil
}

// ParseObjectKey validates the complete key without path cleaning or unescaping.
// Malformed keys must be left untouched by future cleanup callers.
func ParseObjectKey(key string) (ObjectKey, error) {
	// Bound splitting/decoding work before allocating from an untrusted key.
	const maxKeyBytes = len(ObjectPrefix) + (MaxTenantBytes+2)/3*4 + 1 + len("2006/01/02/") + 26 + 1 + len(FormatSpeedscope) + len(".pyrdump")
	if len(key) > maxKeyBytes || !strings.HasPrefix(key, ObjectPrefix) {
		return ObjectKey{}, fmt.Errorf("invalid capture key prefix or length")
	}
	parts := strings.Split(strings.TrimPrefix(key, ObjectPrefix), "/")
	if len(parts) != 5 {
		return ObjectKey{}, fmt.Errorf("invalid capture key shape")
	}
	tenant, err := DecodeTenant(parts[0])
	if err != nil {
		return ObjectKey{}, err
	}
	date := strings.Join(parts[1:4], "/")
	day, err := time.Parse("2006/01/02", date)
	if err != nil || day.Format("2006/01/02") != date {
		return ObjectKey{}, fmt.Errorf("invalid capture date partition")
	}
	file := parts[4]
	if len(file) < 28 || file[26] != '-' || !strings.HasSuffix(file, ".pyrdump") {
		return ObjectKey{}, fmt.Errorf("invalid capture filename")
	}
	id, err := parseCaptureID(file[:26])
	if err != nil {
		return ObjectKey{}, err
	}
	format := Format(strings.TrimSuffix(file[27:], ".pyrdump"))
	if !format.valid() {
		return ObjectKey{}, fmt.Errorf("invalid native format")
	}
	capturedAt := ulid.Time(id.Time()).UTC()
	if err := validateCaptureTime(capturedAt); err != nil {
		return ObjectKey{}, err
	}
	if capturedAt.Format("2006/01/02") != date {
		return ObjectKey{}, fmt.Errorf("capture date partition does not match ULID")
	}
	return ObjectKey{TenantID: tenant, CaptureID: id, CaptureTime: capturedAt, Format: format}, nil
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

func validateCaptureTime(t time.Time) error {
	if t.UTC().Year() < 1970 || t.UTC().Year() > 9999 {
		return fmt.Errorf("capture time must be within years 1970 through 9999 UTC")
	}
	return nil
}
