package heatmap

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	pkgmodel "github.com/grafana/pyroscope/v2/pkg/model"
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
	pb.add(fp, labels1, 100, "profile1", 0, pkgmodel.TraceID{}, 1000)
	pb.add(fp, labels2, 200, "profile2", 0, pkgmodel.TraceID{}, 2000)
	pb.add(fp, labels3, 300, "profile3", 0, pkgmodel.TraceID{}, 3000)

	// Verify we have 3 exemplars
	assert.Equal(t, 3, pb.count())

	// Verify they all share the same labelSet (only common labels)
	var collectedLabels []pkgmodel.Labels
	pb.forEach(func(labels pkgmodel.Labels, ts int64, profileID string, spanID uint64, traceID pkgmodel.TraceID, value int64) {
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
	pb.add(fp, labels, 100, "profile1", 123, pkgmodel.TraceID{}, 1000)
	pb.add(fp, labels, 100, "profile1", 123, pkgmodel.TraceID{}, 2000)
	pb.add(fp, labels, 100, "profile1", 123, pkgmodel.TraceID{}, 3000)

	// Should only have 1 exemplar with summed value
	assert.Equal(t, 1, pb.count())

	var totalValue int64
	pb.forEach(func(labels pkgmodel.Labels, ts int64, profileID string, spanID uint64, traceID pkgmodel.TraceID, value int64) {
		totalValue = value
	})

	assert.Equal(t, int64(6000), totalValue, "Values should be summed")
}

func TestPointsBuilder_SameSpanDifferentTraceIDs(t *testing.T) {
	pb := newPointsBuilder()
	labels := pkgmodel.Labels{
		&typesv1.LabelPair{Name: "service", Value: "api"},
	}
	fp := model.Fingerprint(labels.Hash())
	traceID1 := pkgmodel.TraceID{15: 1}
	traceID2 := pkgmodel.TraceID{15: 2}

	pb.add(fp, labels, 100, "profile1", 123, traceID1, 1000)
	pb.add(fp, labels, 100, "profile1", 123, traceID2, 2000)

	require.Equal(t, 2, pb.count())
	var traceIDs []pkgmodel.TraceID
	pb.forEach(func(labels pkgmodel.Labels, ts int64, profileID string, spanID uint64, traceID pkgmodel.TraceID, value int64) {
		traceIDs = append(traceIDs, traceID)
	})
	assert.Equal(t, []pkgmodel.TraceID{traceID1, traceID2}, traceIDs)
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
	pb.add(fp, labels1, 100, "profile1", 0, pkgmodel.TraceID{}, 1000)
	pb.add(fp, labels2, 200, "profile2", 0, pkgmodel.TraceID{}, 2000)
	pb.add(fp, labels3, 300, "profile3", 0, pkgmodel.TraceID{}, 3000)

	// Verify we have 3 exemplars
	assert.Equal(t, 3, pb.count())

	// All should share the same labelSet with only common labels (service, cluster)
	expectedCommonLabels := pkgmodel.Labels{
		&typesv1.LabelPair{Name: "service", Value: "api"},
		&typesv1.LabelPair{Name: "cluster", Value: "prod"},
	}

	pb.forEach(func(labels pkgmodel.Labels, ts int64, profileID string, spanID uint64, traceID pkgmodel.TraceID, value int64) {
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
	pb.add(fp, labels, 100, "", 0, pkgmodel.TraceID{}, 1000)

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

	pb.add(fp, labels, 100, "profile1", 0, pkgmodel.TraceID{}, 1000)

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

	pb.add(fp, labels, 100, "", 123, pkgmodel.TraceID{}, 1000)

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
	pb.add(fp, labels, 300, "profile3", 0, pkgmodel.TraceID{}, 3000)
	pb.add(fp, labels, 100, "profile1", 0, pkgmodel.TraceID{}, 1000)
	pb.add(fp, labels, 200, "profile2", 0, pkgmodel.TraceID{}, 2000)

	// Verify they come out sorted
	var timestamps []int64
	pb.forEach(func(labels pkgmodel.Labels, ts int64, profileID string, spanID uint64, traceID pkgmodel.TraceID, value int64) {
		timestamps = append(timestamps, ts)
	})

	require.Len(t, timestamps, 3)
	assert.Equal(t, int64(100), timestamps[0])
	assert.Equal(t, int64(200), timestamps[1])
	assert.Equal(t, int64(300), timestamps[2])
}

func TestPointsBuilder_AddAfterIteration(t *testing.T) {
	pb := newPointsBuilder()
	labels := pkgmodel.Labels{&typesv1.LabelPair{Name: "service", Value: "api"}}
	fp := model.Fingerprint(labels.Hash())
	// Exercise every field of the exemplar key and comparator, inserting in
	// reverse order. In particular, labels and values must not be part of the key.
	keys := []exemplarKey{
		{timestamp: 1, spanID: 1, traceID: pkgmodel.TraceID{15: 1}, profileID: "a"},
		{timestamp: 1, spanID: 1, traceID: pkgmodel.TraceID{15: 1}, profileID: "b"},
		{timestamp: 1, spanID: 1, traceID: pkgmodel.TraceID{15: 2}, profileID: "a"},
		{timestamp: 1, spanID: 2, traceID: pkgmodel.TraceID{15: 1}, profileID: "a"},
		{timestamp: 2, spanID: 1, traceID: pkgmodel.TraceID{15: 1}, profileID: "a"},
	}
	for i := len(keys) - 1; i >= 0; i-- {
		k := keys[i]
		pb.add(fp, labels, k.timestamp, k.profileID, k.spanID, k.traceID, 1)
	}
	collect := func(wantValue int64) {
		t.Helper()
		var actual []exemplarKey
		pb.forEach(func(_ pkgmodel.Labels, ts int64, profileID string, spanID uint64, traceID pkgmodel.TraceID, value int64) {
			actual = append(actual, exemplarKey{timestamp: ts, profileID: profileID, spanID: spanID, traceID: traceID})
			require.Equal(t, wantValue, value)
		})
		require.Equal(t, keys, actual)
		require.Equal(t, len(keys), pb.count())
	}
	collect(1)
	for _, k := range keys {
		pb.add(fp, labels, k.timestamp, k.profileID, k.spanID, k.traceID, 2)
	}
	collect(3)
	collect(3)
	// A new exemplar after iteration must also preserve existing indices.
	pb.add(fp, labels, 3, "new", 1, pkgmodel.TraceID{}, 3)
	keys = append(keys, exemplarKey{timestamp: 3, profileID: "new", spanID: 1})
	collect(3)
}

// BenchmarkPointsBuilder includes final iteration so deferred sorting is charged
// to the benchmark as well as insertion and duplicate aggregation.
func BenchmarkPointsBuilder(b *testing.B) {
	labels := pkgmodel.Labels{
		&typesv1.LabelPair{Name: "service_name", Value: "benchmark"},
		&typesv1.LabelPair{Name: "pod", Value: "benchmark-0"},
	}
	fp := model.Fingerprint(labels.Hash())
	for _, size := range []int{1000, 10000, 50000} {
		for _, order := range []string{"ordered", "reverse", "shuffled", "duplicates"} {
			b.Run(fmt.Sprintf("%s/samples=%d", order, size), func(b *testing.B) {
				// Prepare deterministic inputs outside the timed region. Duplicate
				// input has ten samples per unique exemplar, in shuffled order.
				ids := make([]int, size)
				unique := size
				if order == "duplicates" {
					unique = size / 10
				}
				for i := range ids {
					ids[i] = i % unique
					if order == "reverse" {
						ids[i] = size - 1 - i
					}
				}
				if order == "shuffled" || order == "duplicates" {
					rand.New(rand.NewSource(42)).Shuffle(len(ids), func(i, j int) {
						ids[i], ids[j] = ids[j], ids[i]
					})
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					pb := newPointsBuilder()
					for _, id := range ids {
						// Many spans share a timestamp, as in repeated profile rows.
						pb.add(fp, labels, int64(id/100), "", uint64(id+1), pkgmodel.TraceID{15: 1}, 1)
					}
					var total int64
					count := 0
					pb.forEach(func(_ pkgmodel.Labels, _ int64, _ string, _ uint64, _ pkgmodel.TraceID, value int64) {
						total += value
						count++
					})
					if count != unique || total != int64(size) {
						b.Fatalf("got %d exemplars and total %d; want %d and %d", count, total, unique, size)
					}
				}
			})
		}
	}
}
