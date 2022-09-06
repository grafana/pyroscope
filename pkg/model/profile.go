package model

import (
	"strings"

	"github.com/gogo/status"
	"github.com/prometheus/prometheus/model/labels"
	"google.golang.org/grpc/codes"

	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
)

// CompareProfile compares the two profiles.
func CompareProfile(a, b *ingestv1.Profile) int64 {
	if a.Timestamp == b.Timestamp {
		return int64(CompareLabelPairs(a.Labels, b.Labels))
	}
	return a.Timestamp - b.Timestamp
}

// ParseProfileTypeSelector parses the profile selector string.
func ParseProfileTypeSelector(id string) (*commonv1.ProfileType, error) {
	parts := strings.Split(id, ":")

	if len(parts) != 5 && len(parts) != 6 {
		return nil, status.Errorf(codes.InvalidArgument, "profile-type selection must be of the form <name>:<sample-type>:<sample-unit>:<period-type>:<period-unit>(:delta), got(%d): %q", len(parts), id)
	}
	name, sampleType, sampleUnit, periodType, periodUnit := parts[0], parts[1], parts[2], parts[3], parts[4]
	return &commonv1.ProfileType{
		Name:       name,
		ID:         id,
		SampleType: sampleType,
		SampleUnit: sampleUnit,
		PeriodType: periodType,
		PeriodUnit: periodUnit,
	}, nil
}

// SelectorFromProfileType builds a *label.Matcher from an profile type struct
func SelectorFromProfileType(profileType *commonv1.ProfileType) *labels.Matcher {
	return &labels.Matcher{
		Type:  labels.MatchEqual,
		Name:  LabelNameProfileType,
		Value: profileType.Name + ":" + profileType.SampleType + ":" + profileType.SampleUnit + ":" + profileType.PeriodType + ":" + profileType.PeriodUnit,
	}
}
