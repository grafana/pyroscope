package model

import (
	"path/filepath"
	"strings"

	"github.com/cespare/xxhash/v2"
	"github.com/gogo/status"
	"github.com/google/pprof/profile"
	"github.com/prometheus/prometheus/model/labels"
	"google.golang.org/grpc/codes"

	profilev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
)

// CompareProfile compares the two profiles.
func CompareProfile(a, b *ingestv1.Profile) int64 {
	if a.Timestamp == b.Timestamp {
		return int64(CompareLabelPairs(a.Labels, b.Labels))
	}
	return a.Timestamp - b.Timestamp
}

// ParseProfileTypeSelector parses the profile selector string.
func ParseProfileTypeSelector(id string) (*typesv1.ProfileType, error) {
	parts := strings.Split(id, ":")

	if len(parts) != 5 && len(parts) != 6 {
		return nil, status.Errorf(codes.InvalidArgument, "profile-type selection must be of the form <name>:<sample-type>:<sample-unit>:<period-type>:<period-unit>(:delta), got(%d): %q", len(parts), id)
	}
	name, sampleType, sampleUnit, periodType, periodUnit := parts[0], parts[1], parts[2], parts[3], parts[4]
	return &typesv1.ProfileType{
		Name:       name,
		ID:         id,
		SampleType: sampleType,
		SampleUnit: sampleUnit,
		PeriodType: periodType,
		PeriodUnit: periodUnit,
	}, nil
}

// SelectorFromProfileType builds a *label.Matcher from an profile type struct
func SelectorFromProfileType(profileType *typesv1.ProfileType) *labels.Matcher {
	return &labels.Matcher{
		Type:  labels.MatchEqual,
		Name:  LabelNameProfileType,
		Value: profileType.Name + ":" + profileType.SampleType + ":" + profileType.SampleUnit + ":" + profileType.PeriodType + ":" + profileType.PeriodUnit,
	}
}

// SetProfileMetadata sets the metadata on the profile.
func SetProfileMetadata(p *profile.Profile, ty *typesv1.ProfileType) {
	p.SampleType = []*profile.ValueType{{Type: ty.SampleType, Unit: ty.SampleUnit}}
	p.DefaultSampleType = ty.SampleType
	p.PeriodType = &profile.ValueType{Type: ty.PeriodType, Unit: ty.PeriodUnit}
	switch ty.Name {
	case "process_cpu": // todo: this should support other types of cpu profiles
		p.Period = 1000000000
	case "memory":
		p.Period = 512 * 1024
	default:
		p.Period = 1
	}
}

func StacktracePartitionFromProfile(lbls []Labels, p *profilev1.Profile) uint64 {
	return xxhash.Sum64String(stacktracePartitionKeyFromProfile(lbls, p))
}

func stacktracePartitionKeyFromProfile(lbls []Labels, p *profilev1.Profile) string {
	// take the first mapping (which is the main binary's file basename)
	if len(p.Mapping) > 0 {
		if filenameID := p.Mapping[0].Filename; filenameID > 0 {
			if filename := extractMappingFilename(p.StringTable[filenameID]); filename != "" {
				return filename
			}
		}
	}

	// failing that look through the labels for the ServiceName
	if len(lbls) > 0 {
		for _, lbl := range lbls[0] {
			if lbl.Name == LabelNameServiceName {
				return lbl.Value
			}
		}
	}

	return "unknown"
}

func extractMappingFilename(filename string) string {
	// See github.com/google/pprof/profile/profile.go
	// It's unlikely that the main binary mapping is one of them.
	if filename == "" ||
		strings.HasPrefix(filename, "[") ||
		strings.HasPrefix(filename, "linux-vdso") ||
		strings.HasPrefix(filename, "/dev/dri/") {
		return ""
	}
	// Like filepath.ToSlash but doesn't rely on OS.
	n := strings.ReplaceAll(filename, `\`, `/`)
	return strings.TrimSpace(filepath.Base(filepath.Clean(n)))
}
