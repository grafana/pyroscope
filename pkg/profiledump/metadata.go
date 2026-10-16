package profiledump

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

// Metadata limits in UTF-8 bytes. See FORMAT.md for the storage contract.
const (
	MaxMetadataSize = 64 * 1024
	MaxTextBytes    = 1024
	MaxLabels       = 32
	MaxLabelName    = 128
	MaxLabelValue   = 1024
	MaxLabelBytes   = 8192
)

type SourceProtocol string

const SourceConnect SourceProtocol = "connect"

func validatePayloadEncoding(encoding string) error {
	switch encoding {
	case "identity", "gzip", "unknown":
		return nil
	default:
		return fmt.Errorf("invalid payload encoding")
	}
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

// ValidateLabel checks one label against the metadata text limits.
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
