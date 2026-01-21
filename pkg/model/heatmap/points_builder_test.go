package heatmap

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	pkgmodel "github.com/grafana/pyroscope/pkg/model"
)

func TestPointsBuilder_SameFingerprintDifferentExemplars(t *testing.T) {
	// Test that when multiple exemplars have the same fingerprint but different
	// timestamps/profileIDs/spanIDs, they share a labelSet with only common labels
	pb := newPointsBuilder()

	// Create labels with the same fingerprint but different values
	commonLabels := pkgmodel.Labels{
		&typesv1.LabelPair{Name: "service", Value: "api"},
		&typesv1.LabelPair{Name: "cluster", Value: "prod"},
	}

	labels1 := append(commonLabels, &typesv1.LabelPair{Name: "instance", Value: "host1"})
	labels2 := append(commonLabels, &typesv1.LabelPair{Name: "instance", Value: "host2"})
	labels3 := append(commonLabels, &typesv1.LabelPair{Name: "instance", Value: "host3"})

	// All should have the same fingerprint (only common labels)
	fp := model.Fingerprint(commonLabels.Hash())

	// Add three exemplars with the same fingerprint but different timestamps
	pb.add(fp, labels1, 100, "profile1", 0, 1000)
	pb.add(fp, labels2, 200, "profile2", 0, 2000)
	pb.add(fp, labels3, 300, "profile3", 0, 3000)

	// Verify we have 3 exemplars
	assert.Equal(t, 3, pb.count())

	// Verify they all share the same labelSet (only common labels)
	var collectedLabels []pkgmodel.Labels
	pb.forEach(func(labels pkgmodel.Labels, ts int64, profileID string, spanID uint64, value int64) {
		collectedLabels = append(collectedLabels, labels)
	})

	require.Len(t, collectedLabels, 3)

	// All exemplars should have the same labelSet with only common labels
	expectedCommonLabels := commonLabels
	for i, labels := range collectedLabels {
		assert.Equal(t, expectedCommonLabels, labels,
			"Exemplar %d should have only common labels", i)
	}
}

func TestPointsBuilder_DuplicateExemplarsSum(t *testing.T) {
	// Test that duplicate exemplars (same timestamp/profileID/spanID) have their values summed
	pb := newPointsBuilder()

	labels := pkgmodel.Labels{
		&typesv1.LabelPair{Name: "service", Value: "api"},
		&typesv1.LabelPair{Name: "cluster", Value: "prod"},
	}
	fp := model.Fingerprint(labels.Hash())

	// Add the same exemplar 3 times
	pb.add(fp, labels, 100, "profile1", 123, 1000)
	pb.add(fp, labels, 100, "profile1", 123, 2000)
	pb.add(fp, labels, 100, "profile1", 123, 3000)

	// Should only have 1 exemplar with summed value
	assert.Equal(t, 1, pb.count())

	var totalValue int64
	pb.forEach(func(labels pkgmodel.Labels, ts int64, profileID string, spanID uint64, value int64) {
		totalValue = value
	})

	assert.Equal(t, int64(6000), totalValue, "Values should be summed")
}

func TestPointsBuilder_LabelIntersection(t *testing.T) {
	// Test that when exemplars with the same fingerprint have different labels,
	// only the intersection is kept
	pb := newPointsBuilder()

	labels1 := pkgmodel.Labels{
		&typesv1.LabelPair{Name: "service", Value: "api"},
		&typesv1.LabelPair{Name: "cluster", Value: "prod"},
		&typesv1.LabelPair{Name: "instance", Value: "host1"},
		&typesv1.LabelPair{Name: "pod", Value: "pod-1"},
	}

	labels2 := pkgmodel.Labels{
		&typesv1.LabelPair{Name: "service", Value: "api"},
		&typesv1.LabelPair{Name: "cluster", Value: "prod"},
		&typesv1.LabelPair{Name: "instance", Value: "host2"},
		&typesv1.LabelPair{Name: "node", Value: "node-1"},
	}

	labels3 := pkgmodel.Labels{
		&typesv1.LabelPair{Name: "service", Value: "api"},
		&typesv1.LabelPair{Name: "cluster", Value: "prod"},
		&typesv1.LabelPair{Name: "node", Value: "node-2"},
	}

	// Use the same fingerprint for all (simulating same series)
	fp := model.Fingerprint(12345)

	// Add exemplars with different labels
	pb.add(fp, labels1, 100, "profile1", 0, 1000)
	pb.add(fp, labels2, 200, "profile2", 0, 2000)
	pb.add(fp, labels3, 300, "profile3", 0, 3000)

	// Verify we have 3 exemplars
	assert.Equal(t, 3, pb.count())

	// All should share the same labelSet with only common labels (service, cluster)
	expectedCommonLabels := pkgmodel.Labels{
		&typesv1.LabelPair{Name: "service", Value: "api"},
		&typesv1.LabelPair{Name: "cluster", Value: "prod"},
	}

	pb.forEach(func(labels pkgmodel.Labels, ts int64, profileID string, spanID uint64, value int64) {
		assert.Equal(t, expectedCommonLabels, labels,
			"Should only contain labels common to all exemplars")
	})
}

func TestPointsBuilder_EmptyProfileIDAndSpanID(t *testing.T) {
	// Test that exemplars with empty profileID and zero spanID are not added
	pb := newPointsBuilder()

	labels := pkgmodel.Labels{
		&typesv1.LabelPair{Name: "service", Value: "api"},
	}
	fp := model.Fingerprint(labels.Hash())

	// Try to add exemplar with empty profileID and zero spanID
	pb.add(fp, labels, 100, "", 0, 1000)

	// Should have no exemplars
	assert.Equal(t, 0, pb.count())
}

func TestPointsBuilder_OnlyProfileID(t *testing.T) {
	// Test that exemplars with only profileID (no spanID) are added
	pb := newPointsBuilder()

	labels := pkgmodel.Labels{
		&typesv1.LabelPair{Name: "service", Value: "api"},
	}
	fp := model.Fingerprint(labels.Hash())

	pb.add(fp, labels, 100, "profile1", 0, 1000)

	// Should have 1 exemplar
	assert.Equal(t, 1, pb.count())
}

func TestPointsBuilder_OnlySpanID(t *testing.T) {
	// Test that exemplars with only spanID (no profileID) are added
	pb := newPointsBuilder()

	labels := pkgmodel.Labels{
		&typesv1.LabelPair{Name: "service", Value: "api"},
	}
	fp := model.Fingerprint(labels.Hash())

	pb.add(fp, labels, 100, "", 123, 1000)

	// Should have 1 exemplar
	assert.Equal(t, 1, pb.count())
}

func TestPointsBuilder_Sorting(t *testing.T) {
	// Test that exemplars are sorted by timestamp, then spanID, then profileID
	pb := newPointsBuilder()

	labels := pkgmodel.Labels{
		&typesv1.LabelPair{Name: "service", Value: "api"},
	}
	fp := model.Fingerprint(labels.Hash())

	// Add exemplars in non-sorted order
	pb.add(fp, labels, 300, "profile3", 0, 3000)
	pb.add(fp, labels, 100, "profile1", 0, 1000)
	pb.add(fp, labels, 200, "profile2", 0, 2000)

	// Verify they come out sorted
	var timestamps []int64
	pb.forEach(func(labels pkgmodel.Labels, ts int64, profileID string, spanID uint64, value int64) {
		timestamps = append(timestamps, ts)
	})

	require.Len(t, timestamps, 3)
	assert.Equal(t, int64(100), timestamps[0])
	assert.Equal(t, int64(200), timestamps[1])
	assert.Equal(t, int64(300), timestamps[2])
}
