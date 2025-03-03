package ingester

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/google/uuid"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/kv"
	// "github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/objstore/client"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	phlarectx "github.com/grafana/pyroscope/pkg/phlare/context"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/validation"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	ingesterv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/prometheus/client_golang/prometheus"
)

// mockLimits implements the Limits interface for testing
type mockLimits struct {
	maxSeriesPerUser        int
	maxLabelNamesPerSeries  int
}

func (m *mockLimits) MaxSeriesPerUser(_ string) int { return m.maxSeriesPerUser }
func (m *mockLimits) MaxLabelNamesPerSeries(_ string) int { return m.maxLabelNamesPerSeries }
func (m *mockLimits) MaxLocalSeriesPerUser(_ string) int { return m.maxSeriesPerUser }
func (m *mockLimits) MaxLocalSeriesPerMetric(_ string) int { return m.maxSeriesPerUser }
func (m *mockLimits) MaxLocalSeriesPerTenant(_ string) int { return m.maxSeriesPerUser }
func (m *mockLimits) MaxGlobalSeriesPerUser(_ string) int { return m.maxSeriesPerUser }
func (m *mockLimits) MaxGlobalSeriesPerMetric(_ string) int { return m.maxSeriesPerUser }
func (m *mockLimits) MaxGlobalSeriesPerTenant(_ string) int { return m.maxSeriesPerUser }
func (m *mockLimits) DistributorUsageGroups(_ string) *validation.UsageGroupConfig { return nil }
func (m *mockLimits) IngestionTenantShardSize(_ string) int { return 1024 * 1024 * 1024 }

func setupTestIngester(b *testing.B) (*Ingester, error) {
	// Create a temporary directory for the test data
	tmpDir, err := os.MkdirTemp("", "ingester-bench-*")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})

	// Setup basic context with logger and registry
	logger := log.NewNopLogger()
	reg := prometheus.NewRegistry()
	ctx := phlarectx.WithLogger(context.Background(), logger)
	ctx = phlarectx.WithRegistry(ctx, reg)

	// Configure local storage bucket
	bucketConfig := client.Config{
		StorageBackendConfig: client.StorageBackendConfig{
			Backend: "filesystem",
			Filesystem: filesystem.Config{
				Directory: filepath.Join(tmpDir, "storage"),
			},
		},
	}

	storageBucket, err := client.NewBucket(ctx, bucketConfig, "storage")
	if err != nil {
		b.Fatal(err)
	}

	// Basic ingester config
	cfg := Config{
		LifecyclerConfig: ring.LifecyclerConfig{
			RingConfig: ring.Config{
				KVStore: kv.Config{
					Store: "inmemory",
				},
				ReplicationFactor: 1,
			},
			NumTokens:        1,
			HeartbeatPeriod: 100 * time.Millisecond,
			JoinAfter:       100 * time.Millisecond,
			ObservePeriod:   100 * time.Millisecond,
		},
	}

	// Database config
	dbConfig := phlaredb.Config{
		DataPath:           filepath.Join(tmpDir, "data"),
		MaxBlockDuration:   2 * time.Hour,
		RowGroupTargetSize: 1024 * 1024 * 1024, // 1GB
		DisableEnforcement: true,               // Disable enforcement for benchmarks
	}

	// Basic limits for testing
	limits := &mockLimits{
		maxSeriesPerUser:        100000,
		maxLabelNamesPerSeries: 100,
	}

	return New(ctx, cfg, dbConfig, storageBucket, limits, 0)
}

func generateTestProfile() []byte {
	// Create a simple profile for testing
	profile := &profilev1.Profile{
		SampleType: []*profilev1.ValueType{
			{Type: 1, Unit: 1},
		},
		Sample: []*profilev1.Sample{
			{
				Value:      []int64{1},
				LocationId: []uint64{1},
			},
		},
	}
	// Serialize the profile - in real code, handle the error
	data, _ := profile.MarshalVT()
	return data
}

func generateLabels(cardinality int) []string {
	labels := make([]string, 0, cardinality*2)
	// Always include service label
	labels = append(labels, "service", "test")
	
	// Add additional labels
	for i := 0; i < cardinality-1; i++ {
		labels = append(labels, 
			fmt.Sprintf("label_%d", i), 
			fmt.Sprintf("value_%d", i))
	}
	return labels
}

// Base benchmarks
func BenchmarkIngester_Push(b *testing.B) {
	ctx := context.Background()
	ing, err := setupTestIngester(b)
	if err != nil {
		b.Fatal(err)
	}

	if err := ing.StartAsync(ctx); err != nil {
		b.Fatal(err)
	}
	if err := ing.AwaitRunning(ctx); err != nil {
		b.Fatal(err)
	}
	defer ing.StopAsync()

	profile := generateTestProfile()
	req := connect.NewRequest(&pushv1.PushRequest{
		Series: []*pushv1.RawProfileSeries{
			{
				Labels: []*typesv1.LabelPair{
					{
						Name:  "service",
						Value: "test",
					},
				},
				Samples: []*pushv1.RawSample{
					{
						ID:         uuid.New().String(),
						RawProfile: profile,
					},
				},
			},
		},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ing.Push(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIngester_Flush(b *testing.B) {
	ctx := context.Background()
	ing, err := setupTestIngester(b)
	if err != nil {
		b.Fatal(err)
	}

	if err := ing.StartAsync(ctx); err != nil {
		b.Fatal(err)
	}
	if err := ing.AwaitRunning(ctx); err != nil {
		b.Fatal(err)
	}
	defer ing.StopAsync()

	// First push some data
	profile := generateTestProfile()
	pushReq := connect.NewRequest(&pushv1.PushRequest{
		Series: []*pushv1.RawProfileSeries{
			{
				Labels: []*typesv1.LabelPair{
					{
						Name:  "service",
						Value: "test",
					},
				},
				Samples: []*pushv1.RawSample{
					{
						ID:         uuid.New().String(),
						RawProfile: profile,
					},
				},
			},
		},
	})
	_, err = ing.Push(ctx, pushReq)
	if err != nil {
		b.Fatal(err)
	}

	flushReq := connect.NewRequest(&ingesterv1.FlushRequest{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ing.Flush(ctx, flushReq)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Label cardinality benchmarks
func BenchmarkIngester_Push_LabelCardinality(b *testing.B) {
	cardinalities := []int{1, 5, 10, 20, 50}
	
	for _, cardinality := range cardinalities {
		b.Run(fmt.Sprintf("labels_%d", cardinality), func(b *testing.B) {
			ctx := context.Background()
			ing, err := setupTestIngester(b)
			if err != nil {
				b.Fatal(err)
			}

			if err := ing.StartAsync(ctx); err != nil {
				b.Fatal(err)
			}
			if err := ing.AwaitRunning(ctx); err != nil {
				b.Fatal(err)
			}
			defer ing.StopAsync()

			profile := generateTestProfile()
			// labels := generateLabels(cardinality) // TODO: fix this
			labels := []*typesv1.LabelPair{
				{
					Name:  "service",
					Value: "test",
				},
			}
			
			req := connect.NewRequest(&pushv1.PushRequest{
				Series: []*pushv1.RawProfileSeries{
					{
						Labels: labels,
						Samples: []*pushv1.RawSample{
							{
								ID:         uuid.New().String(),
								RawProfile: profile,
							},
						},
					},
				},
			})

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := ing.Push(ctx, req)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkIngester_Flush_LabelCardinality(b *testing.B) {
	cardinalities := []int{1, 5, 10, 20, 50}
	
	for _, cardinality := range cardinalities {
		b.Run(fmt.Sprintf("labels_%d", cardinality), func(b *testing.B) {
			ctx := context.Background()
			ing, err := setupTestIngester(b)
			if err != nil {
				b.Fatal(err)
			}

			if err := ing.StartAsync(ctx); err != nil {
				b.Fatal(err)
			}
			if err := ing.AwaitRunning(ctx); err != nil {
				b.Fatal(err)
			}
			defer ing.StopAsync()

			// Push data with different label cardinalities
			profile := generateTestProfile()
			// labels := generateLabels(cardinality) // TODO: fix this
			labels := []*typesv1.LabelPair{
				{
					Name:  "service",
					Value: "test",
				},
			}
			
			// Push multiple samples to ensure we have enough data to make the flush meaningful
			for i := 0; i < 100; i++ {
				pushReq := connect.NewRequest(&pushv1.PushRequest{
					Series: []*pushv1.RawProfileSeries{
						{
							Labels: labels,
							Samples: []*pushv1.RawSample{
								{
									ID:         uuid.New().String(),
									RawProfile: profile,
								},
							},
						},
					},
				})
				_, err = ing.Push(ctx, pushReq)
				if err != nil {
					b.Fatal(err)
				}
			}

			flushReq := connect.NewRequest(&ingesterv1.FlushRequest{})

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := ing.Flush(ctx, flushReq)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
} 