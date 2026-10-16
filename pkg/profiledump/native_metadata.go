package profiledump

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
	"unicode/utf8"

	"github.com/oklog/ulid/v2"
)

const NativeSchemaVersion = 1

// NativeMetadata describes a capture in its JSON sidecar.
type NativeMetadata struct {
	SchemaVersion     uint16            `json:"schema_version"`
	CapturedAt        time.Time         `json:"captured_at"`
	TenantID          string            `json:"tenant_id"`
	SourceProtocol    SourceProtocol    `json:"source_protocol"`
	NativeFormat      Format            `json:"native_format"`
	PayloadEncoding   string            `json:"payload_encoding"`
	Labels            map[string]string `json:"labels,omitempty"`
	OriginalProfileID string            `json:"original_profile_id,omitempty"`
	DistributorID     string            `json:"distributor_id"`
	CaptureID         string            `json:"capture_id"`
	PayloadSize       int64             `json:"payload_size"`
}

// Validate checks bounded fields and identity against a complete native key.
func (m NativeMetadata) Validate(key string) error {
	parsed, err := ParseNativeObjectKey(key)
	if err != nil {
		return err
	}
	if m.SchemaVersion != NativeSchemaVersion {
		return fmt.Errorf("unsupported native metadata schema version %d", m.SchemaVersion)
	}
	if m.TenantID != parsed.TenantID || m.CaptureID != parsed.CaptureID.String() {
		return fmt.Errorf("metadata identity does not match native key")
	}
	if err := validateCaptureTime(m.CapturedAt); err != nil {
		return err
	}
	if ulid.Timestamp(m.CapturedAt) != parsed.CaptureID.Time() {
		return fmt.Errorf("captured_at does not match native key")
	}
	if m.SourceProtocol != SourceConnect || m.NativeFormat != FormatPprof {
		return fmt.Errorf("native metadata requires connect/pprof provenance")
	}
	if err := validatePayloadEncoding(m.PayloadEncoding); err != nil {
		return err
	}
	if m.PayloadSize < 0 {
		return fmt.Errorf("payload_size must be nonnegative")
	}
	if err := ValidateOriginalProfileID(m.OriginalProfileID); err != nil {
		return err
	}
	if err := validateText("distributor_id", m.DistributorID, MaxTextBytes, true); err != nil {
		return err
	}
	return ValidateLabels(m.Labels)
}

// MarshalNativeMetadata validates before allocating JSON, marshals once, and
// checks the actual serialized size, including JSON escaping expansion.
func MarshalNativeMetadata(key string, metadata NativeMetadata) ([]byte, error) {
	if err := metadata.Validate(key); err != nil {
		return nil, err
	}
	return marshalValidatedNativeMetadata(metadata)
}

// marshalValidatedNativeMetadata requires validation before allocating JSON.
// It retains the final size check for JSON escaping expansion.
func marshalValidatedNativeMetadata(metadata NativeMetadata) ([]byte, error) {
	metadata.CapturedAt = metadata.CapturedAt.UTC()
	b, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("encode native metadata: %w", err)
	}
	if len(b) > MaxMetadataSize {
		return nil, fmt.Errorf("native metadata exceeds %d bytes", MaxMetadataSize)
	}
	return b, nil
}

// ReadNativeMetadata reads at most MaxMetadataSize+1 bytes and validates exactly
// one JSON sidecar against its native key.
func ReadNativeMetadata(r io.Reader, key string) (NativeMetadata, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxMetadataSize+1))
	if err != nil {
		return NativeMetadata{}, fmt.Errorf("read native metadata: %w", err)
	}
	if len(b) == 0 || len(b) > MaxMetadataSize {
		return NativeMetadata{}, fmt.Errorf("native metadata length must be in [1, %d]", MaxMetadataSize)
	}
	if !utf8.Valid(b) {
		return NativeMetadata{}, fmt.Errorf("native metadata is not UTF-8")
	}
	// A pointer distinguishes missing/null payload_size from a valid empty payload.
	var wire struct {
		NativeMetadata
		PayloadSize *int64 `json:"payload_size"`
	}
	if err := json.Unmarshal(b, &wire); err != nil {
		return NativeMetadata{}, fmt.Errorf("decode native metadata: %w", err)
	}
	if wire.PayloadSize == nil {
		return NativeMetadata{}, fmt.Errorf("payload_size is required")
	}
	wire.NativeMetadata.PayloadSize = *wire.PayloadSize
	if err := wire.Validate(key); err != nil {
		return NativeMetadata{}, fmt.Errorf("invalid native metadata: %w", err)
	}
	return wire.NativeMetadata, nil
}
