package model

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cespare/xxhash/v2"
	"github.com/gogo/status"
	"github.com/prometheus/prometheus/model/labels"
	"google.golang.org/grpc/codes"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/util"
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

type SpanSelector map[uint64]struct{}

func NewSpanSelector(spans []string) (SpanSelector, error) {
	m := make(map[uint64]struct{}, len(spans))
	b := make([]byte, 8)
	for _, s := range spans {
		if len(s) != 16 {
			return nil, fmt.Errorf("invalid span id length: %q", s)
		}
		if _, err := hex.Decode(b, util.YoloBuf(s)); err != nil {
			return nil, err
		}
		m[binary.LittleEndian.Uint64(b)] = struct{}{}
	}
	return m, nil
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
