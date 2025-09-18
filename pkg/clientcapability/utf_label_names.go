package clientcapability

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/grafana/pyroscope/pkg/validation"
)

func Utf8LabelNamesEnabled(ctx context.Context) bool {
	if capabilities, ok := GetClientCapabilities(ctx); ok {
		if val, ok := capabilities[AllowUtf8LabelNamesCapabilityName]; ok && val == "true" {
			return true
		}
	}
	return false
}

// SanitizeLabelNames uses legacy logic to transform non utf-8 compliant label names
// (if possible). It will error on duplicate label names post-sanitization.
func SanitizeLabelNames(labelNames []string) ([]string, error) {
	slices.Sort(labelNames)
	var (
		lastLabelName = ""
	)
	for idx, labelName := range labelNames {
		if origName, newName, ok := sanitizeLabelName(labelName); ok && origName != newName {
			labelNames[idx] = newName
			lastLabelName = origName
			continue
		} else if !ok {
			return nil, validation.NewErrorf(validation.InvalidLabels, validation.InvalidLabelsErrorMsg, labelName,
				fmt.Sprintf("invalid label name '%s'. consider setting '%s=true' in the `Accept` header", origName, AllowUtf8LabelNamesCapabilityName))
		}

		if cmp := strings.Compare(lastLabelName, labelName); cmp == 0 {
			return nil, validation.NewErrorf(validation.DuplicateLabelNames, validation.DuplicateLabelNamesErrorMsg, lastLabelName, labelName)
		}
		lastLabelName = labelName
	}

	return labelNames, nil
}

// sanitizeLabelName reports whether the label name is valid,
// and returns the sanitized value.
//
// The only change the function makes is replacing dots with underscores.
func sanitizeLabelName(ln string) (old, sanitized string, ok bool) {
	if len(ln) == 0 {
		return ln, ln, false
	}
	hasDots := false
	for i, b := range ln {
		if (b < 'a' || b > 'z') && (b < 'A' || b > 'Z') && b != '_' && (b < '0' || b > '9' || i == 0) {
			if b == '.' {
				hasDots = true
			} else {
				return ln, ln, false
			}
		}
	}
	if !hasDots {
		return ln, ln, true
	}
	r := []rune(ln)
	for i, b := range r {
		if b == '.' {
			r[i] = '_'
		}
	}
	return ln, string(r), true
}
