package memdb

import (
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

// BenchmarkWriteProfilesArrow benchmarks the Arrow-based implementation
func BenchmarkWriteProfilesArrow(b *testing.B) {
	profiles := generateBenchmarkArrowProfiles(1000, 100)
	metrics := NewHeadMetricsWithPrefix(prometheus.NewRegistry(), "")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := WriteProfilesArrow(metrics, profiles)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWriteProfiles benchmarks the traditional implementation (now using Arrow internally)
func BenchmarkWriteProfiles(b *testing.B) {
	profiles := generateBenchmarkArrowProfiles(1000, 100)
	metrics := NewHeadMetricsWithPrefix(prometheus.NewRegistry(), "")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := WriteProfiles(metrics, profiles)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkInMemoryArrowProfilesToRecord benchmarks the Arrow conversion
func BenchmarkInMemoryArrowProfilesToRecord(b *testing.B) {
	profiles := generateBenchmarkArrowProfiles(1000, 100)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		record, err := inMemoryArrowProfilesToRecord(profiles)
		if err != nil {
			b.Fatal(err)
		}
		record.Release()
	}
}

// BenchmarkArrowRecordToInMemoryProfiles benchmarks the reverse conversion
func BenchmarkArrowRecordToInMemoryProfiles(b *testing.B) {
	profiles := generateBenchmarkArrowProfiles(1000, 100)
	record, err := inMemoryArrowProfilesToRecord(profiles)
	if err != nil {
		b.Fatal(err)
	}
	defer record.Release()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := arrowRecordToInMemoryProfiles(record)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSerializeArrowRecord benchmarks Arrow serialization
func BenchmarkSerializeArrowRecord(b *testing.B) {
	profiles := generateBenchmarkArrowProfiles(1000, 100)
	record, err := inMemoryArrowProfilesToRecord(profiles)
	if err != nil {
		b.Fatal(err)
	}
	defer record.Release()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := serializeArrowRecord(record)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWriteProfilesArrowSmall benchmarks with small profiles
func BenchmarkWriteProfilesArrowSmall(b *testing.B) {
	profiles := generateBenchmarkArrowProfiles(100, 10)
	metrics := NewHeadMetricsWithPrefix(prometheus.NewRegistry(), "")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := WriteProfilesArrow(metrics, profiles)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWriteProfilesSmall benchmarks traditional with small profiles
func BenchmarkWriteProfilesSmall(b *testing.B) {
	profiles := generateBenchmarkArrowProfiles(100, 10)
	metrics := NewHeadMetricsWithPrefix(prometheus.NewRegistry(), "")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := WriteProfiles(metrics, profiles)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWriteProfilesArrowLarge benchmarks with large profiles
func BenchmarkWriteProfilesArrowLarge(b *testing.B) {
	profiles := generateBenchmarkArrowProfiles(10000, 1000)
	metrics := NewHeadMetricsWithPrefix(prometheus.NewRegistry(), "")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := WriteProfilesArrow(metrics, profiles)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWriteProfilesLarge benchmarks traditional with large profiles
func BenchmarkWriteProfilesLarge(b *testing.B) {
	profiles := generateBenchmarkArrowProfiles(10000, 1000)
	metrics := NewHeadMetricsWithPrefix(prometheus.NewRegistry(), "")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := WriteProfiles(metrics, profiles)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMemoryUsage compares memory usage between implementations
func BenchmarkMemoryUsage(b *testing.B) {
	profiles := generateBenchmarkArrowProfiles(1000, 100)
	metrics := NewHeadMetricsWithPrefix(prometheus.NewRegistry(), "")

	b.Run("Arrow", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := WriteProfilesArrow(metrics, profiles)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Traditional", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := WriteProfiles(metrics, profiles)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkConvertInMemoryProfileToArrow benchmarks the conversion function
func BenchmarkConvertInMemoryProfileToArrow(b *testing.B) {
	traditionalProfiles := generateBenchmarkTraditionalProfiles(1000, 100)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		arrowProfiles := ConvertInMemoryProfilesToArrow(traditionalProfiles)
		_ = arrowProfiles
	}
}

// Helper function to generate benchmark traditional profiles
func generateBenchmarkTraditionalProfiles(numProfiles, samplesPerProfile int) []v1.InMemoryProfile {
	profiles := make([]v1.InMemoryProfile, numProfiles)

	for i := 0; i < numProfiles; i++ {
		profile := v1.InMemoryProfile{
			ID:                  uuid.New(),
			SeriesIndex:         uint32(i),
			StacktracePartition: uint64(i * 1000),
			TotalValue:          uint64(samplesPerProfile * 100),
			SeriesFingerprint:   model.Fingerprint(i * 10000),
			DropFrames:          int64(i * 10),
			KeepFrames:          int64(i * 20),
			TimeNanos:           int64(i * 1000000000),
			DurationNanos:       int64(i * 1000000),
			Period:              int64(i * 1000),
			DefaultSampleType:   int64(i),
			Samples:             v1.NewSamples(samplesPerProfile),
			Annotations: v1.Annotations{
				Keys:   []string{"service", "version"},
				Values: []string{"test-service", "v1.0.0"},
			},
			Comments: []int64{int64(i * 100), int64(i * 200)},
		}

		// Fill samples
		for j := 0; j < samplesPerProfile; j++ {
			profile.Samples.StacktraceIDs[j] = uint32(i*1000 + j)
			profile.Samples.Values[j] = uint64(i*100 + j)
		}

		profiles[i] = profile
	}

	return profiles
}

// Helper function to generate benchmark Arrow profiles
func generateBenchmarkArrowProfiles(numProfiles, samplesPerProfile int) []InMemoryArrowProfile {
	profiles := make([]InMemoryArrowProfile, numProfiles)

	for i := 0; i < numProfiles; i++ {
		profile := InMemoryArrowProfile{
			ID:                  uuid.New(),
			SeriesIndex:         uint32(i),
			StacktracePartition: uint64(i * 1000),
			TotalValue:          uint64(samplesPerProfile * 100),
			SeriesFingerprint:   model.Fingerprint(i * 10000),
			DropFrames:          int64(i * 10),
			KeepFrames:          int64(i * 20),
			TimeNanos:           int64(i * 1000000000),
			DurationNanos:       int64(i * 1000000),
			Period:              int64(i * 1000),
			DefaultSampleType:   int64(i),
			Comments:            []int64{int64(i * 100), int64(i * 200)},
			StacktraceIDs:       make([]uint32, samplesPerProfile),
			Values:              make([]uint64, samplesPerProfile),
			Spans:               make([]uint64, samplesPerProfile),
			AnnotationKeys:      []string{"service", "version"},
			AnnotationValues:    []string{"test-service", "v1.0.0"},
		}

		// Fill samples
		for j := 0; j < samplesPerProfile; j++ {
			profile.StacktraceIDs[j] = uint32(i*1000 + j)
			profile.Values[j] = uint64(i*100 + j)
			profile.Spans[j] = uint64(i*10000 + j)
		}

		profiles[i] = profile
	}

	return profiles
}
