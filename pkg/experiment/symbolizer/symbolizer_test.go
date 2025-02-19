package symbolizer

import (
	"context"
	"fmt"
	"strings"
	"testing"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

// TestSymbolizeTree tests symbolization using testdata/symbols.debug which contains:
//
// 0x1500 ->
//
//	 main (/usr/src/stress-1.0.7-1/src/stress.c:87)
//		fprintf (/usr/include/x86_64-linux-gnu/bits/stdio2.h:77)
//
// 0x3c5a -> atoll_b (/usr/src/stress-1.0.7-1/src/stress.c:632)
// 0x2745 -> main (/usr/src/stress-1.0.7-1/src/stress.c:87)
func TestSymbolizeTree(t *testing.T) {
	tests := []struct {
		name     string
		profile  *googlev1.Profile
		buildID  string
		wantErr  bool
		validate func(*testing.T, *googlev1.Profile)
	}{
		{
			name: "already symbolized mapping",
			profile: &googlev1.Profile{
				Mapping: []*googlev1.Mapping{{
					HasFunctions:   true,
					HasFilenames:   true,
					HasLineNumbers: true,
				}},
				Location: []*googlev1.Location{{
					MappingId: 1,
					Line: []*googlev1.Line{{
						FunctionId: 0,
						Line:       42,
					}},
				}},
				Function: []*googlev1.Function{{
					Name:     1,
					Filename: 2,
				}},
				StringTable: []string{"", "main", "main.go"},
			},
			validate: func(t *testing.T, p *googlev1.Profile) {
				require.True(t, p.Mapping[0].HasFunctions)
				require.True(t, p.Mapping[0].HasFilenames)
				require.True(t, p.Mapping[0].HasLineNumbers)
			},
		},
		{
			name: "needs symbolization",
			profile: &googlev1.Profile{
				Mapping: []*googlev1.Mapping{{
					BuildId:     1,
					MemoryStart: 0x0,
					MemoryLimit: 0x1000000,
					FileOffset:  0x0,
				}},
				Location: []*googlev1.Location{{
					MappingId: 1,
					Address:   0x1500,
				}},
				StringTable: []string{"", "build-id"},
			},
			buildID: "build-id",
			validate: func(t *testing.T, p *googlev1.Profile) {
				require.True(t, p.Mapping[0].HasFunctions)
				require.True(t, p.Mapping[0].HasFilenames)
				require.True(t, p.Mapping[0].HasLineNumbers)

				// Validate first location has two lines (main and fprintf)
				require.Len(t, p.Location[0].Line, 2)

				// Check main function
				mainFunc := p.Function[p.Location[0].Line[0].FunctionId]
				require.Equal(t, "main", p.StringTable[mainFunc.Name])
				require.Equal(t, "/usr/src/stress-1.0.7-1/src/stress.c", p.StringTable[mainFunc.Filename])
				require.Equal(t, int64(87), p.Location[0].Line[0].Line)
				require.Equal(t, int64(86), mainFunc.StartLine)

				// Check fprintf function
				fprintfFunc := p.Function[p.Location[0].Line[1].FunctionId]
				require.Equal(t, "fprintf", p.StringTable[fprintfFunc.Name])
				require.Equal(t, "/usr/include/x86_64-linux-gnu/bits/stdio2.h", p.StringTable[fprintfFunc.Filename])
				require.Equal(t, int64(77), p.Location[0].Line[1].Line)
				require.Equal(t, int64(77), fprintfFunc.StartLine)
			},
		},
		{
			name: "invalid function references",
			profile: &googlev1.Profile{
				Mapping: []*googlev1.Mapping{{
					BuildId:      1,
					HasFunctions: true, // Incorrectly set
				}},
				Location: []*googlev1.Location{{
					MappingId: 1,
					Line: []*googlev1.Line{{
						FunctionId: 999, // Invalid reference
					}},
				}},
				StringTable: []string{"", "build-id"},
			},
			buildID: "build-id",
			validate: func(t *testing.T, p *googlev1.Profile) {
				// Should detect invalid function reference and fix mapping flags
				require.False(t, p.Mapping[0].HasFunctions)
			},
		},
		{
			name: "empty build ID",
			profile: &googlev1.Profile{
				Mapping: []*googlev1.Mapping{{
					BuildId: 1,
				}},
				StringTable: []string{"", ""},
			},
			validate: func(t *testing.T, p *googlev1.Profile) {
				require.False(t, p.Mapping[0].HasFunctions)
			},
		},
		{
			name: "multiple locations per mapping",
			profile: &googlev1.Profile{
				Mapping: []*googlev1.Mapping{{
					BuildId:     1,
					MemoryStart: 0x0,
					MemoryLimit: 0x1000000,
					FileOffset:  0x0,
				}},
				Location: []*googlev1.Location{
					{MappingId: 1, Address: 0x1500},
					{MappingId: 1, Address: 0x3c5a},
					{MappingId: 1, Address: 0x2745},
				},
				StringTable: []string{"", "build-id"},
			},
			buildID: "build-id",
			validate: func(t *testing.T, p *googlev1.Profile) {
				require.True(t, p.Mapping[0].HasFunctions)

				// First location (0x1500) - main and fprintf
				require.Len(t, p.Location[0].Line, 2)
				mainFunc := p.Function[p.Location[0].Line[0].FunctionId]
				require.Equal(t, "main", p.StringTable[mainFunc.Name])

				// Second location (0x3c5a) - atoll_b
				require.Len(t, p.Location[1].Line, 1)
				atollFunc := p.Function[p.Location[1].Line[0].FunctionId]
				require.Equal(t, "atoll_b", p.StringTable[atollFunc.Name])
				require.Equal(t, int64(632), p.Location[1].Line[0].Line)

				// Third location (0x2745) - main
				require.Len(t, p.Location[2].Line, 1)
				mainFunc2 := p.Function[p.Location[2].Line[0].FunctionId]
				require.Equal(t, "main", p.StringTable[mainFunc2.Name])
				require.Equal(t, int64(87), p.Location[2].Line[0].Line)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockDebuginfodClient{buildID: tt.buildID}
			s := NewProfileSymbolizer(mockClient, NewNullCache(), NewMetrics(nil))

			// Marshal profile into tree report
			data, err := tt.profile.MarshalVT()
			require.NoError(t, err)
			report := &queryv1.TreeReport{Tree: data}

			// Run symbolization
			err = s.SymbolizeTree(context.Background(), report)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Unmarshal result for validation
			result := &googlev1.Profile{}
			err = result.UnmarshalVT(report.Tree)
			require.NoError(t, err)

			tt.validate(t, result)
		})
	}
}

func TestSymbolizerMetrics(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*ProfileSymbolizer, context.Context) error
		expected    string
		metricNames []string
	}{
		{
			name: "successful symbolization",
			setup: func(s *ProfileSymbolizer, ctx context.Context) error {
				profile := &googlev1.Profile{
					Mapping: []*googlev1.Mapping{{
						BuildId:     1,
						MemoryStart: 0x0,
						MemoryLimit: 0x1000000,
					}},
					Location: []*googlev1.Location{{
						MappingId: 1,
						Address:   0x1500,
					}},
					StringTable: []string{"", "build-id"},
				}
				data, _ := profile.MarshalVT()
				return s.SymbolizeTree(ctx, &queryv1.TreeReport{Tree: data})
			},
			expected: `
		        # HELP pyroscope_symbolizer_tree_requests_total Total number of tree symbolization requests
		        # TYPE pyroscope_symbolizer_tree_requests_total counter
		        pyroscope_symbolizer_tree_requests_total 1

		        # HELP pyroscope_symbolizer_locations_total Total number of locations processed
		        # TYPE pyroscope_symbolizer_locations_total counter
		        pyroscope_symbolizer_locations_total{status="success"} 1

		        # HELP pyroscope_symbolizer_internal_errors_total Total number of internal symbolization errors
		        # TYPE pyroscope_symbolizer_internal_errors_total counter
		        pyroscope_symbolizer_internal_errors_total{reason="success"} 1
		    `,
			metricNames: []string{
				"pyroscope_symbolizer_tree_requests_total",
				"pyroscope_symbolizer_locations_total",
			},
		},
		{
			name: "unmarshal error",
			setup: func(s *ProfileSymbolizer, ctx context.Context) error {
				return s.SymbolizeTree(ctx, &queryv1.TreeReport{Tree: []byte("invalid")})
			},
			expected: `
				# HELP pyroscope_symbolizer_tree_errors_total Total number of tree symbolization errors
				# TYPE pyroscope_symbolizer_tree_errors_total counter
				pyroscope_symbolizer_tree_errors_total{reason="unmarshal_error"} 1
			`,
			metricNames: []string{
				"pyroscope_symbolizer_tree_errors_total",
			},
		},
		{
			name: "debuginfod error",
			setup: func(s *ProfileSymbolizer, ctx context.Context) error {
				profile := &googlev1.Profile{
					Mapping: []*googlev1.Mapping{{
						BuildId: 1,
					}},
					Location: []*googlev1.Location{{
						MappingId: 1,
						Address:   0x1500,
					}},
					StringTable: []string{"", "unknown-build-id"},
				}
				data, _ := profile.MarshalVT()
				return s.SymbolizeTree(ctx, &queryv1.TreeReport{Tree: data})
			},
			expected: `
				# HELP pyroscope_symbolizer_tree_errors_total Total number of tree symbolization errors
				# TYPE pyroscope_symbolizer_tree_errors_total counter
				pyroscope_symbolizer_tree_errors_total{reason="symbolization_error"} 1
		
				# HELP pyroscope_symbolizer_internal_errors_total Total number of internal symbolization errors
				# TYPE pyroscope_symbolizer_internal_errors_total counter
				pyroscope_symbolizer_internal_errors_total{reason="debuginfod_error"} 1
			`,
			metricNames: []string{
				"pyroscope_symbolizer_tree_errors_total",
				"pyroscope_symbolizer_internal_errors_total",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := prometheus.NewRegistry()
			metrics := NewMetrics(reg)
			s := NewProfileSymbolizer(&mockDebuginfodClient{buildID: "build-id"}, NewNullCache(), metrics)

			err := tt.setup(s, context.Background())
			if err != nil {
				t.Log("Setup error:", err)
			}

			err = testutil.GatherAndCompare(reg, strings.NewReader(tt.expected), tt.metricNames...)
			require.NoError(t, err)
		})
	}
}

type mockDebuginfodClient struct {
	buildID string
}

func (m *mockDebuginfodClient) FetchDebuginfo(buildID string) (string, error) {
	if buildID != m.buildID {
		return "", fmt.Errorf("unknown build ID")
	}

	return "testdata/symbols.debug", nil
}
