package querier

import (
	"testing"

	"github.com/stretchr/testify/assert"

	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

func Test_getDataFromPlan(t *testing.T) {
	tests := []struct {
		name                       string
		plan                       blockPlan
		wantIngesterQueryScope     *queryScope
		wantStoreGatewayQueryScope *queryScope
		wantDeduplicationNeeded    bool
	}{
		{
			name: "empty plan",
			plan: blockPlan{},
			wantIngesterQueryScope: &queryScope{
				QueryScope: &querierv1.QueryScope{
					ComponentType: "Short term storage",
				},
			},
			wantStoreGatewayQueryScope: &queryScope{
				QueryScope: &querierv1.QueryScope{
					ComponentType: "Long term storage",
				},
			},
			wantDeduplicationNeeded: false,
		},
		{
			name: "plan with ingesters only",
			plan: blockPlan{
				"replica 1": &blockPlanEntry{
					BlockHints:   &ingestv1.BlockHints{Ulids: []string{"block A", "block B"}, Deduplication: true},
					InstanceType: ingesterInstance,
				},
				"replica 2": &blockPlanEntry{
					BlockHints:   &ingestv1.BlockHints{Ulids: []string{"block C", "block D"}, Deduplication: true},
					InstanceType: ingesterInstance,
				},
			},
			wantIngesterQueryScope: &queryScope{
				QueryScope: &querierv1.QueryScope{
					ComponentType:  "Short term storage",
					ComponentCount: 2,
					NumBlocks:      4,
				},
				blockIds: []string{"block A", "block B", "block C", "block D"},
			},
			wantStoreGatewayQueryScope: &queryScope{
				QueryScope: &querierv1.QueryScope{
					ComponentType: "Long term storage",
				},
			},
			wantDeduplicationNeeded: true,
		},
		{
			name: "plan with ingesters and store gateways",
			plan: blockPlan{
				"replica 1": &blockPlanEntry{
					BlockHints:   &ingestv1.BlockHints{Ulids: []string{"block A", "block B"}, Deduplication: true},
					InstanceType: ingesterInstance,
				},
				"replica 2": &blockPlanEntry{
					BlockHints:   &ingestv1.BlockHints{Ulids: []string{"block C", "block D"}, Deduplication: true},
					InstanceType: ingesterInstance,
				},
				"replica 3": &blockPlanEntry{
					BlockHints:   &ingestv1.BlockHints{Ulids: []string{"block E", "block F"}, Deduplication: true},
					InstanceType: storeGatewayInstance,
				},
			},
			wantIngesterQueryScope: &queryScope{
				QueryScope: &querierv1.QueryScope{
					ComponentType:  "Short term storage",
					ComponentCount: 2,
					NumBlocks:      4,
				},
				blockIds: []string{"block A", "block B", "block C", "block D"},
			},
			wantStoreGatewayQueryScope: &queryScope{
				QueryScope: &querierv1.QueryScope{
					ComponentType:  "Long term storage",
					ComponentCount: 1,
					NumBlocks:      2,
				},
				blockIds: []string{"block E", "block F"},
			},
			wantDeduplicationNeeded: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIngesterQueryScope, gotStoreGatewayQueryScope, gotDeduplicationNeeded := getDataFromPlan(tt.plan)
			assert.Equalf(t, tt.wantIngesterQueryScope, gotIngesterQueryScope, "getDataFromPlan(%v)", tt.plan)
			assert.Equalf(t, tt.wantStoreGatewayQueryScope, gotStoreGatewayQueryScope, "getDataFromPlan(%v)", tt.plan)
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
				assert.Equalf(t, uint64(0), s.NumSeries, "addBlockStatsToQueryScope(%v)", s)
				assert.Equalf(t, uint64(0), s.NumProfiles, "addBlockStatsToQueryScope(%v)", s)
				assert.Equalf(t, uint64(0), s.NumSamples, "addBlockStatsToQueryScope(%v)", s)
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
									NumSeries:     50,
									NumProfiles:   100,
									NumSamples:    2000,
									IndexBytes:    1024,
									ProfilesBytes: 4096,
									SymbolsBytes:  65536,
								},
								{
									NumSeries:     100,
									NumProfiles:   200,
									NumSamples:    4000,
									IndexBytes:    2048,
									ProfilesBytes: 8192,
									SymbolsBytes:  131072,
								},
							},
						},
					},
					{
						addr: "replica 2",
						response: &ingestv1.GetBlockStatsResponse{
							BlockStats: []*ingestv1.BlockStats{
								{
									NumSeries:     50,
									NumProfiles:   100,
									NumSamples:    2000,
									IndexBytes:    1024,
									ProfilesBytes: 4096,
									SymbolsBytes:  65536,
								},
							},
						},
					},
				},
				queryScope: &queryScope{QueryScope: &querierv1.QueryScope{}},
			},
			verifyExpectations: func(t *testing.T, s *queryScope) {
				assert.Equalf(t, uint64(200), s.NumSeries, "addBlockStatsToQueryScope(%v)", s)
				assert.Equalf(t, uint64(400), s.NumProfiles, "addBlockStatsToQueryScope(%v)", s)
				assert.Equalf(t, uint64(8000), s.NumSamples, "addBlockStatsToQueryScope(%v)", s)
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
