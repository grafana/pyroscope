package model

import ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"

// CompareProfile compares the two profiles.
func CompareProfile(a, b *ingestv1.Profile) int64 {
	if a.Timestamp == b.Timestamp {
		return int64(CompareLabelPair(a.Labels, b.Labels))
	}
	return a.Timestamp - b.Timestamp
}
