package querier

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

func Test_getDataFromPlan(t *testing.T) {
	tests := []struct {
		name                         string
		plan                         blockPlan
		verifyIngesterQueryScope     func(t *testing.T, scope *queryScope)
		verifyStoreGatewayQueryScope func(t *testing.T, scope *queryScope)
		wantDeduplicationNeeded      bool
	}{
		{
			name: "empty plan",
			plan: blockPlan{},
			verifyIngesterQueryScope: func(t *testing.T, scope *queryScope) {
				require.Equal(t, &queryScope{
					QueryScope: &querierv1.QueryScope{
						ComponentType: "Short term storage",
					},
				}, scope)
			},
			verifyStoreGatewayQueryScope: func(t *testing.T, scope *queryScope) {
				require.Equal(t, &queryScope{
					QueryScope: &querierv1.QueryScope{
						ComponentType: "Long term storage",
					},
				}, scope)
			},
			wantDeduplicationNeeded: false,
		},
		{
			name: "plan with ingesters only",
			plan: blockPlan{
				"replica 1": &blockPlanEntry{
					BlockHints:    &ingestv1.BlockHints{Ulids: []string{"block A", "block B"}, Deduplication: true},
					InstanceTypes: []instanceType{ingesterInstance},
				},
				"replica 2": &blockPlanEntry{
					BlockHints:    &ingestv1.BlockHints{Ulids: []string{"block C", "block D"}, Deduplication: true},
					InstanceTypes: []instanceType{ingesterInstance},
				},
			},
			verifyIngesterQueryScope: func(t *testing.T, scope *queryScope) {
				require.Equal(t, uint64(2), scope.ComponentCount)
				require.Equal(t, uint64(4), scope.BlockCount)
				for _, block := range []string{"block A", "block B", "block C", "block D"} {
					require.True(t, slices.Contains(scope.blockIds, block))
				}
			},
			verifyStoreGatewayQueryScope: func(t *testing.T, scope *queryScope) {
				require.Equal(t, uint64(0), scope.ComponentCount)
			},
			wantDeduplicationNeeded: true,
		},
		{
			name: "plan with ingesters and store gateways",
			plan: blockPlan{
				"replica 1": &blockPlanEntry{
					BlockHints:    &ingestv1.BlockHints{Ulids: []string{"block A", "block B"}, Deduplication: true},
					InstanceTypes: []instanceType{ingesterInstance},
				},
				"replica 2": &blockPlanEntry{
					BlockHints:    &ingestv1.BlockHints{Ulids: []string{"block C", "block D"}, Deduplication: true},
					InstanceTypes: []instanceType{ingesterInstance},
				},
				"replica 3": &blockPlanEntry{
					BlockHints:    &ingestv1.BlockHints{Ulids: []string{"block E", "block F"}, Deduplication: true},
					InstanceTypes: []instanceType{storeGatewayInstance},
				},
			},
			verifyIngesterQueryScope: func(t *testing.T, scope *queryScope) {
				require.Equal(t, uint64(2), scope.ComponentCount)
				require.Equal(t, uint64(4), scope.BlockCount)
				for _, block := range []string{"block A", "block B", "block C", "block D"} {
					require.True(t, slices.Contains(scope.blockIds, block))
				}
			},
			verifyStoreGatewayQueryScope: func(t *testing.T, scope *queryScope) {
				require.Equal(t, uint64(1), scope.ComponentCount)
				require.Equal(t, uint64(2), scope.BlockCount)
				for _, block := range []string{"block E", "block F"} {
					require.True(t, slices.Contains(scope.blockIds, block))
				}
			},
			wantDeduplicationNeeded: true,
		},
		{
			name: "plan with a single replica with dual instance types (standalone binary)",
			plan: blockPlan{
				"replica 1": &blockPlanEntry{
					BlockHints:    &ingestv1.BlockHints{Ulids: []string{"block A"}, Deduplication: true},
					InstanceTypes: []instanceType{ingesterInstance, storeGatewayInstance},
				},
			},
			verifyIngesterQueryScope: func(t *testing.T, scope *queryScope) {
				require.Equal(t, uint64(1), scope.ComponentCount)
				require.Equal(t, uint64(1), scope.BlockCount)
				for _, block := range []string{"block A"} {
					require.True(t, slices.Contains(scope.blockIds, block))
				}
			},
			verifyStoreGatewayQueryScope: func(t *testing.T, scope *queryScope) {
				require.Equal(t, uint64(1), scope.ComponentCount)
				require.Equal(t, uint64(1), scope.BlockCount)
				for _, block := range []string{"block A"} {
					require.True(t, slices.Contains(scope.blockIds, block))
				}
			},
			wantDeduplicationNeeded: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIngesterQueryScope, gotStoreGatewayQueryScope, gotDeduplicationNeeded := getDataFromPlan(tt.plan)
			tt.verifyIngesterQueryScope(t, gotIngesterQueryScope)
			tt.verifyStoreGatewayQueryScope(t, gotStoreGatewayQueryScope)
			assert.Equalf(t, tt.wantDeduplicationNeeded, gotDeduplicationNeeded, "getDataFromPlan(%v)", tt.plan)
		})
	}
}

func Test_addBlockStatsToQueryScope(t *testing.T) {
	type args struct {
		blockStatsFromReplicas []ResponseFromReplica[*ingestv1.GetBlockStatsResponse]
		queryScope             *queryScope
	}
	tests := []struct {
		name               string
		args               args
		verifyExpectations func(t *testing.T, s *queryScope)
	}{
		{
			name: "with empty block stats",
			args: args{
				blockStatsFromReplicas: []ResponseFromReplica[*ingestv1.GetBlockStatsResponse]{},
				queryScope:             &queryScope{QueryScope: &querierv1.QueryScope{}},
			},
			verifyExpectations: func(t *testing.T, s *queryScope) {
				assert.Equalf(t, uint64(0), s.SeriesCount, "addBlockStatsToQueryScope(%v)", s)
				assert.Equalf(t, uint64(0), s.ProfileCount, "addBlockStatsToQueryScope(%v)", s)
				assert.Equalf(t, uint64(0), s.SampleCount, "addBlockStatsToQueryScope(%v)", s)
				assert.Equalf(t, uint64(0), s.IndexBytes, "addBlockStatsToQueryScope(%v)", s)
				assert.Equalf(t, uint64(0), s.ProfileBytes, "addBlockStatsToQueryScope(%v)", s)
				assert.Equalf(t, uint64(0), s.SymbolBytes, "addBlockStatsToQueryScope(%v)", s)
			},
		},
		{
			name: "with valid block stats",
			args: args{
				blockStatsFromReplicas: []ResponseFromReplica[*ingestv1.GetBlockStatsResponse]{
					{
						addr: "replica 1",
						response: &ingestv1.GetBlockStatsResponse{
							BlockStats: []*ingestv1.BlockStats{
								{
									SeriesCount:  50,
									ProfileCount: 100,
									SampleCount:  2000,
									IndexBytes:   1024,
									ProfileBytes: 4096,
									SymbolBytes:  65536,
								},
								{
									SeriesCount:  100,
									ProfileCount: 200,
									SampleCount:  4000,
									IndexBytes:   2048,
									ProfileBytes: 8192,
									SymbolBytes:  131072,
								},
							},
						},
					},
					{
						addr: "replica 2",
						response: &ingestv1.GetBlockStatsResponse{
							BlockStats: []*ingestv1.BlockStats{
								{
									SeriesCount:  50,
									ProfileCount: 100,
									SampleCount:  2000,
									IndexBytes:   1024,
									ProfileBytes: 4096,
									SymbolBytes:  65536,
								},
							},
						},
					},
				},
				queryScope: &queryScope{QueryScope: &querierv1.QueryScope{}},
			},
			verifyExpectations: func(t *testing.T, s *queryScope) {
				assert.Equalf(t, uint64(200), s.SeriesCount, "addBlockStatsToQueryScope(%v)", s)
				assert.Equalf(t, uint64(400), s.ProfileCount, "addBlockStatsToQueryScope(%v)", s)
				assert.Equalf(t, uint64(8000), s.SampleCount, "addBlockStatsToQueryScope(%v)", s)
				assert.Equalf(t, uint64(4096), s.IndexBytes, "addBlockStatsToQueryScope(%v)", s)
				assert.Equalf(t, uint64(16384), s.ProfileBytes, "addBlockStatsToQueryScope(%v)", s)
				assert.Equalf(t, uint64(262144), s.SymbolBytes, "addBlockStatsToQueryScope(%v)", s)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addBlockStatsToQueryScope(tt.args.blockStatsFromReplicas, tt.args.queryScope)
			tt.verifyExpectations(t, tt.args.queryScope)
		})
	}
}

func Test_createResponse(t *testing.T) {
	type args struct {
		ingesterQueryScope     *queryScope
		storeGatewayQueryScope *queryScope
	}
	tests := []struct {
		name string
		args args
		want *querierv1.AnalyzeQueryResponse
	}{
		{
			name: "happy path",
			args: args{
				ingesterQueryScope: &queryScope{
					QueryScope: &querierv1.QueryScope{
						IndexBytes:   1024,
						ProfileBytes: 2048,
						SymbolBytes:  4096,
					},
				},
				storeGatewayQueryScope: &queryScope{
					QueryScope: &querierv1.QueryScope{
						IndexBytes:   256,
						ProfileBytes: 512,
						SymbolBytes:  1024,
					},
				},
			},
			want: &querierv1.AnalyzeQueryResponse{
				QueryScopes: []*querierv1.QueryScope{
					{
						IndexBytes:   1024,
						ProfileBytes: 2048,
						SymbolBytes:  4096,
					},
					{
						IndexBytes:   256,
						ProfileBytes: 512,
						SymbolBytes:  1024,
					},
				},
				QueryImpact: &querierv1.QueryImpact{
					TotalBytesInTimeRange: 8960,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, createResponse(tt.args.ingesterQueryScope, tt.args.storeGatewayQueryScope), "createResponse(%v, %v)", tt.args.ingesterQueryScope, tt.args.storeGatewayQueryScope)
		})
	}
}

func Test_createMatchersFromQuery(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		want    []string
		wantErr bool
	}{
		{
			name:    "empty query",
			query:   "",
			want:    []string{"{}"},
			wantErr: false,
		},
		{
			name:    "query with a profile type",
			query:   "process_cpu:cpu:nanoseconds:cpu:nanoseconds{}",
			want:    []string{"{__profile_type__=\"process_cpu:cpu:nanoseconds:cpu:nanoseconds\"}"},
			wantErr: false,
		},
		{
			name:    "query with labels",
			query:   "process_cpu:cpu:nanoseconds:cpu:nanoseconds{namespace=\"dev\"}",
			want:    []string{"{namespace=\"dev\",__profile_type__=\"process_cpu:cpu:nanoseconds:cpu:nanoseconds\"}"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := createMatchersFromQuery(tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equalf(t, tt.want, got, "createMatchersFromQuery(%v)", tt.query)
		})
	}
}
