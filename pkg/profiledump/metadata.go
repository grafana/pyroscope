package profiledump

import (
	"fmt"
	"mime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/oklog/ulid/v2"
)

// Codec limits in UTF-8 bytes. See FORMAT.md for the wire contract.
const (
	MaxLegacyTextBytes = 4096
	MaxMetadataSize    = 64 * 1024
	MaxTenantBytes     = 256
	MaxTextBytes       = 1024
	MaxLabels          = 32
	MaxLabelName       = 128
	MaxLabelValue      = 1024
	MaxLabelBytes      = 8192
)

type SourceProtocol string

const (
	SourceConnect  SourceProtocol = "connect"
	SourceIngest   SourceProtocol = "ingest"
	SourceOTLPHTTP SourceProtocol = "otlp_http"
	SourceOTLPGRPC SourceProtocol = "otlp_grpc"

	ActivationRuntimeOverride = "runtime_override"
)

// Representation describes incoming or stored bytes, retaining content-type parameters.
// Encoding is "unknown" when the remaining encoding cannot be determined.
type Representation struct {
	ContentType string `json:"content_type"`
	Encoding    string `json:"encoding"` // identity, gzip, zstd, deflate, br, snappy, or unknown
	Syntax      string `json:"syntax"`   // protobuf, json, multipart, binary, or text
}

const (
	encodingIdentity = "identity"
	encodingUnknown  = "unknown"
)

const syntaxMultipart = "multipart"

// LegacyMetadata holds parsed /ingest values and client profile times.
// Labels live in Metadata.Labels. Omitted DeclaredFormat is empty.
type LegacyMetadata struct {
	Name           string    `json:"name"`
	StartTime      time.Time `json:"start_time"`
	EndTime        time.Time `json:"end_time"`
	SampleRate     uint32    `json:"sample_rate"`
	Spy            string    `json:"spy"`
	Units          string    `json:"units"`
	Aggregation    string    `json:"aggregation"`
	DeclaredFormat string    `json:"declared_format"`
}

func (m LegacyMetadata) validate() error {
	total := 0
	for _, field := range []struct{ name, value string }{
		{"name", m.Name}, {"spy", m.Spy}, {"units", m.Units},
		{"aggregation", m.Aggregation}, {"declared_format", m.DeclaredFormat},
	} {
		if err := validateText(field.name, field.value, MaxTextBytes, false); err != nil {
			return err
		}
		total += len(field.value)
	}
	if total > MaxLegacyTextBytes {
		return fmt.Errorf("legacy metadata exceeds byte limit")
	}
	for _, t := range []time.Time{m.StartTime, m.EndTime} {
		if _, err := t.MarshalJSON(); err != nil {
			return fmt.Errorf("invalid legacy profile time: %w", err)
		}
	}
	return nil
}

// Metadata describes a capture. Labels and payloads remain sensitive.
// SchemaVersion keeps metadata self-describing outside its envelope.
// Callers must exclude credentials and arbitrary request data.
type Metadata struct {
	SchemaVersion     uint16            `json:"schema_version"`
	CapturedAt        time.Time         `json:"captured_at"`
	TenantID          string            `json:"tenant_id"`
	SourceProtocol    SourceProtocol    `json:"source_protocol"`
	NativeFormat      Format            `json:"native_format"`
	Incoming          *Representation   `json:"incoming,omitempty"`
	Stored            Representation    `json:"stored"`
	Labels            map[string]string `json:"labels,omitempty"`
	Legacy            *LegacyMetadata   `json:"legacy,omitempty"`
	HTTP              *HTTPMetadata     `json:"http,omitempty"`
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
	case SourceConnect, SourceIngest, SourceOTLPHTTP, SourceOTLPGRPC:
	default:
		return fmt.Errorf("invalid source protocol")
	}
	if !m.NativeFormat.valid() {
		return fmt.Errorf("invalid native format")
	}
	if m.Incoming != nil {
		if err := m.Incoming.validate(); err != nil {
			return fmt.Errorf("incoming representation: %w", err)
		}
	}
	if err := m.Stored.validate(); err != nil {
		return fmt.Errorf("stored representation: %w", err)
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
	if m.HTTP != nil && (m.SourceProtocol != SourceOTLPHTTP && m.SourceProtocol != SourceIngest) {
		return fmt.Errorf("HTTP metadata requires an HTTP source")
	}
	if m.Legacy != nil {
		if m.SourceProtocol != SourceIngest {
			return fmt.Errorf("legacy metadata requires ingest source")
		}
		if err := m.Legacy.validate(); err != nil {
			return err
		}
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

// ValidateOriginalProfileID checks an optional ID. Invalid IDs may be omitted.
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

func (r Representation) validate() error {
	if err := validateText("content_type", r.ContentType, MaxTextBytes, true); err != nil {
		return err
	}
	mediaType, params, err := mime.ParseMediaType(r.ContentType)
	if err != nil {
		return fmt.Errorf("invalid content type: %w", err)
	}
	multipart := strings.HasPrefix(mediaType, "multipart/")
	if multipart && (params["boundary"] == "" || r.Syntax != syntaxMultipart) {
		return fmt.Errorf("multipart content type requires boundary and multipart syntax")
	}
	if r.Syntax == syntaxMultipart && !multipart {
		return fmt.Errorf("multipart syntax requires multipart content type")
	}
	switch r.Encoding {
	case encodingIdentity, "gzip", "zstd", "deflate", "br", "snappy", encodingUnknown:
	default:
		return fmt.Errorf("invalid content encoding")
	}
	switch r.Syntax {
	case "protobuf", "json", syntaxMultipart, "binary", "text":
	default:
		return fmt.Errorf("invalid representation syntax")
	}
	return nil
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
