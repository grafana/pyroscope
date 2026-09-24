package profiledump

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/oklog/ulid/v2"
)

// Codec limits in UTF-8 bytes. See FORMAT.md for the wire contract.
const (
	MaxMetadataSize = 64 * 1024
	MaxTenantBytes  = 256
	MaxTextBytes    = 1024
	MaxLabels       = 32
	MaxLabelName    = 128
	MaxLabelValue   = 1024
	MaxLabelBytes   = 8192
)

type SourceProtocol string

const SourceConnect SourceProtocol = "connect"

const ActivationRuntimeOverride = "runtime_override"

// Metadata describes a capture independently of its envelope.
type Metadata struct {
	SchemaVersion     uint16            `json:"schema_version"`
	CapturedAt        time.Time         `json:"captured_at"`
	TenantID          string            `json:"tenant_id"`
	SourceProtocol    SourceProtocol    `json:"source_protocol"`
	NativeFormat      Format            `json:"native_format"`
	PayloadEncoding   string            `json:"payload_encoding"`
	Labels            map[string]string `json:"labels,omitempty"`
	OriginalProfileID string            `json:"original_profile_id,omitempty"`
	DistributorID     string            `json:"distributor_id"`
	ActivationSource  string            `json:"activation_source"`
	PolicyFingerprint string            `json:"policy_fingerprint"`
	CaptureID         string            `json:"capture_id"`
	PayloadSize       int64             `json:"payload_size"`
}

// Validate checks fields. Encode also enforces serialized metadata and object limits.
func (m Metadata) Validate() error {
	if err := m.validateSchema(); err != nil {
		return err
	}
	if err := m.validateCaptureIdentity(); err != nil {
		return err
	}
	switch m.SourceProtocol {
	case SourceConnect:
	default:
		return fmt.Errorf("invalid source protocol")
	}
	if !m.NativeFormat.valid() {
		return fmt.Errorf("invalid native format")
	}
	switch m.PayloadEncoding {
	case "identity", "gzip", "unknown":
	default:
		return fmt.Errorf("invalid payload encoding")
	}
	if err := ValidateOriginalProfileID(m.OriginalProfileID); err != nil {
		return err
	}
	if err := validateText("distributor_id", m.DistributorID, MaxTextBytes, true); err != nil {
		return err
	}
	if m.ActivationSource != ActivationRuntimeOverride {
		return fmt.Errorf("invalid activation source")
	}
	if err := validatePolicyFingerprint(m.PolicyFingerprint); err != nil {
		return err
	}
	if m.PayloadSize < 0 {
		return fmt.Errorf("payload_size must be nonnegative")
	}
	return ValidateLabels(m.Labels)
}

func (m Metadata) validateSchema() error {
	if m.SchemaVersion != Version {
		return fmt.Errorf("unsupported metadata schema version %d", m.SchemaVersion)
	}

	return nil
}

func (m Metadata) validateCaptureIdentity() error {
	if err := validateCaptureTime(m.CapturedAt); err != nil {
		return err
	}
	id, err := parseCaptureID(m.CaptureID)
	if err != nil {
		return err
	}
	if id.Time() != ulid.Timestamp(m.CapturedAt) {
		return fmt.Errorf("capture ID timestamp does not match captured_at")
	}
	if err := validateText("tenant_id", m.TenantID, MaxTenantBytes, true); err != nil {
		return err
	}
	return nil
}

func validatePolicyFingerprint(fingerprint string) error {
	if len(fingerprint) != 64 {
		return fmt.Errorf("policy fingerprint must be 64 lowercase hexadecimal characters")
	}
	for _, c := range fingerprint {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return fmt.Errorf("invalid policy fingerprint")
		}
	}
	return nil
}

// ValidateOriginalProfileID checks an optional ID.
func ValidateOriginalProfileID(id string) error {
	return validateText("original_profile_id", id, MaxTextBytes, false)
}

// ValidateLabels checks a complete map without copying or sorting it.
func ValidateLabels(values map[string]string) error {
	if len(values) > MaxLabels {
		return fmt.Errorf("too many selected labels")
	}
	labelBytes := 0
	for name, value := range values {
		if err := ValidateLabel(name, value); err != nil {
			return err
		}
		labelBytes += len(name) + len(value) // Individual sizes and count are already bounded.
	}
	if labelBytes > MaxLabelBytes {
		return fmt.Errorf("selected labels exceed byte limit")
	}
	return nil
}

// ValidateLabel checks one label against the envelope's text limits.
func ValidateLabel(name, value string) error {
	if err := validateText("label name", name, MaxLabelName, true); err != nil {
		return err
	}
	return validateText("label value", value, MaxLabelValue, false)
}

func validateText(field, s string, maxBytes int, required bool) error {
	if len(s) > maxBytes || required && len(s) == 0 || !utf8.ValidString(s) {
		return fmt.Errorf("invalid %s length or UTF-8", field)
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return fmt.Errorf("control character in %s", field)
		}
	}
	return nil
}
