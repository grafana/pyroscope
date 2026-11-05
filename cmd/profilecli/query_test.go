package main

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockquerierv1connect"
)

func TestQueryPprofWithFallback(t *testing.T) {
	client := mockquerierv1connect.NewMockQuerierServiceClient(t)
	req := &querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
		LabelSelector: "{}",
		Start:         1,
		End:           2,
		Format:        querierv1.ProfileFormat_PROFILE_FORMAT_PPROF,
	}
	client.On("SelectMergeStacktraces", mock.Anything, connect.NewRequest(req)).
		Return(connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{
			Flamegraph: &querierv1.FlameGraph{},
		}), nil).Once()
	want := &profilev1.Profile{Sample: []*profilev1.Sample{{Value: []int64{1}}}}
	client.On("SelectMergeProfile", mock.Anything, mock.MatchedBy(func(req *connect.Request[querierv1.SelectMergeProfileRequest]) bool {
		return req.Msg.ProfileTypeID == "process_cpu:cpu:nanoseconds:cpu:nanoseconds" && req.Msg.Start == 1 && req.Msg.End == 2
	})).Return(connect.NewResponse(want), nil).Once()

	got, _, err := queryPprofWithFallback(context.Background(), client, req)

	require.NoError(t, err)
	require.Same(t, want, got)
}

func TestQueryProfileParams_ProfileIDValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		profileIDs []string
		wantErr    bool
	}{
		{
			name:       "valid UUID",
			profileIDs: []string{"550e8400-e29b-41d4-a716-446655440000"},
			wantErr:    false,
		},
		{
			name:       "valid UUID v4",
			profileIDs: []string{uuid.New().String()},
			wantErr:    false,
		},
		{
			name:       "multiple valid UUIDs",
			profileIDs: []string{"550e8400-e29b-41d4-a716-446655440000", uuid.New().String()},
			wantErr:    false,
		},
		{
			name:       "invalid UUID - span ID",
			profileIDs: []string{"deadbeef12345678"},
			wantErr:    true,
		},
		{
			name:       "invalid UUID - random string",
			profileIDs: []string{"not-a-uuid"},
			wantErr:    true,
		},
		{
			name:       "one valid one invalid",
			profileIDs: []string{"550e8400-e29b-41d4-a716-446655440000", "not-a-uuid"},
			wantErr:    true,
		},
		{
			name:       "empty slice is valid (not provided)",
			profileIDs: nil,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			params := &queryProfileParams{ProfileIDs: tt.profileIDs}
			err := validateQueryProfileParams(params)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "--profile-id must be a valid UUID")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestQueryProfileParams_MutualExclusion(t *testing.T) {
	t.Parallel()

	// --profile-id and --span-selector cannot be used together.
	err := validateQueryProfileParams(&queryProfileParams{
		ProfileIDs:   []string{"550e8400-e29b-41d4-a716-446655440000"},
		SpanSelector: []string{"deadbeef12345678"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--profile-id and --span-selector cannot be used together")

	// No conflict when only profile-id is set.
	err = validateQueryProfileParams(&queryProfileParams{
		ProfileIDs: []string{"550e8400-e29b-41d4-a716-446655440000"},
	})
	require.NoError(t, err)

	// No conflict when only span-selector is set.
	err = validateQueryProfileParams(&queryProfileParams{
		SpanSelector: []string{"deadbeef12345678"},
	})
	require.NoError(t, err)

	// --span-selector and --stacktrace-selector cannot be used together.
	err = validateQueryProfileParams(&queryProfileParams{
		SpanSelector:       []string{"deadbeef12345678"},
		StacktraceSelector: []string{"main"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--span-selector and --stacktrace-selector cannot be used together")

	// --trace-id and --span-selector cannot be used together.
	err = validateQueryProfileParams(&queryProfileParams{
		TraceIDs:     []string{"0123456789abcdef0123456789abcdef"},
		SpanSelector: []string{"deadbeef12345678"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--trace-id and --span-selector cannot be used together")

	// --trace-id and --profile-id cannot be used together.
	err = validateQueryProfileParams(&queryProfileParams{
		TraceIDs:   []string{"0123456789abcdef0123456789abcdef"},
		ProfileIDs: []string{"550e8400-e29b-41d4-a716-446655440000"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--trace-id and --profile-id cannot be used together")
}

func TestQueryProfileParams_TraceIDValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		traceIDs []string
		wantErr  bool
	}{
		{name: "valid 32-char hex", traceIDs: []string{"0123456789abcdef0123456789abcdef"}, wantErr: false},
		{name: "multiple valid", traceIDs: []string{"0123456789abcdef0123456789abcdef", "ffffffffffffffffffffffffffffffff"}, wantErr: false},
		{name: "too short (span id)", traceIDs: []string{"deadbeef12345678"}, wantErr: true},
		{name: "invalid hex", traceIDs: []string{"0123456789abcdef0123456789abcdeg"}, wantErr: true},
		{name: "empty slice is valid", traceIDs: nil, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateQueryProfileParams(&queryProfileParams{TraceIDs: tt.traceIDs})
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "--trace-id must be a 32-character hex string")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
