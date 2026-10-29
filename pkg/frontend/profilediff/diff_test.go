package profilediff

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func TestDiffJoinsBeforeLimiting(t *testing.T) {
	req := &querierv1.DiffRequest{
		Left: &querierv1.SelectMergeStacktracesRequest{
			ProfileTypeID: "cpu",
			LabelSelector: "left",
			Format:        querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS,
			FormatOptions: &querierv1.ProfileFormatOptions{
				Functions: &querierv1.FunctionsOptions{Limit: 1},
			},
		},
		Right: &querierv1.SelectMergeStacktracesRequest{
			ProfileTypeID: "cpu",
			LabelSelector: "right",
			Format:        querierv1.ProfileFormat_PROFILE_FORMAT_FLAMEGRAPH,
			FormatOptions: &querierv1.ProfileFormatOptions{
				Functions: &querierv1.FunctionsOptions{Limit: -1},
			},
		},
	}
	original := req.CloneVT()
	resp, err := Diff(context.Background(), req, func(_ context.Context, r *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
		require.Zero(t, r.Msg.GetFormatOptions().GetFunctions().Limit)
		require.Equal(t, req.Left.Format, r.Msg.Format)
		table := &typesv1.FunctionTable{
			Total:          100,
			TotalFunctions: 2,
			Functions: []*typesv1.FunctionStats{{
				Name:  "A",
				Self:  60,
				Total: 60,
			}, {
				Name:  "B",
				Self:  40,
				Total: 40,
			}},
		}
		if r.Msg.LabelSelector == "right" {
			table = &typesv1.FunctionTable{
				Total:          150,
				TotalFunctions: 2,
				Functions: []*typesv1.FunctionStats{{
					Name:  "B",
					Self:  80,
					Total: 80,
				}, {
					Name:  "C",
					Self:  70,
					Total: 70,
				}},
			}
		}
		return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{Functions: table}), nil
	})
	require.NoError(t, err)
	require.Equal(t, int64(100), resp.Msg.Functions.Left.Total)
	require.Equal(t, int64(150), resp.Msg.Functions.Right.Total)
	require.Equal(t, int64(3), resp.Msg.Functions.Left.TotalFunctions)
	require.Len(t, resp.Msg.Functions.Left.Functions, 1)
	require.Equal(t, "B", resp.Msg.Functions.Left.Functions[0].Name)
	require.Equal(t, int64(40), resp.Msg.Functions.Left.Functions[0].Self)
	require.Equal(t, int64(80), resp.Msg.Functions.Right.Functions[0].Self)
	require.True(t, original.EqualVT(req))
}

func TestJoinFunctionsNilTables(t *testing.T) {
	full := &typesv1.FunctionTable{
		Total:          10,
		TotalFunctions: 1,
		Functions: []*typesv1.FunctionStats{{
			Name:  "A",
			Total: 10,
			Self:  6,
		}},
	}
	zero := &typesv1.FunctionTable{
		TotalFunctions: 1,
		Functions:      []*typesv1.FunctionStats{{Name: "A"}},
	}
	for _, tc := range []struct {
		name  string
		left  *typesv1.FunctionTable
		right *typesv1.FunctionTable
		want  *querierv1.FunctionTableDiff
	}{
		{"both nil", nil, nil, &querierv1.FunctionTableDiff{
			Left:  &typesv1.FunctionTable{},
			Right: &typesv1.FunctionTable{},
		}},
		{"left nil", nil, full, &querierv1.FunctionTableDiff{
			Left:  zero,
			Right: full,
		}},
		{"right nil", full, nil, &querierv1.FunctionTableDiff{
			Left:  full,
			Right: zero,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := joinFunctions(tc.left, tc.right, 0)
			require.True(t, tc.want.EqualVT(got), "want %v, got %v", tc.want, got)
			if tc.left != nil {
				require.Same(t, tc.left.Functions[0], got.Left.Functions[0])
			}
			if tc.right != nil {
				require.Same(t, tc.right.Functions[0], got.Right.Functions[0])
			}
		})
	}
}

func TestJoinFunctionsReusesRowsWithoutMutatingInputs(t *testing.T) {
	left := &typesv1.FunctionTable{
		Functions: []*typesv1.FunctionStats{
			{
				Name:  "B",
				Self:  4,
				Total: 10,
			},
			{
				Name:  "shared",
				Self:  8,
				Total: 12,
			},
		},
	}
	right := &typesv1.FunctionTable{
		Functions: []*typesv1.FunctionStats{
			{
				Name:  "A",
				Self:  4,
				Total: 10,
			},
			{
				Name:  "shared",
				Self:  2,
				Total: 6,
			},
		},
	}
	originalLeft, originalRight := left.CloneVT(), right.CloneVT()
	for _, limit := range []int64{0, 2, 1 << 40} {
		got := joinFunctions(left, right, limit)
		require.Same(t, left.Functions[1], got.Left.Functions[0])
		require.Same(t, right.Functions[1], got.Right.Functions[0])
		require.Same(t, right.Functions[0], got.Right.Functions[1])
		require.Equal(t, &typesv1.FunctionStats{Name: "A"}, got.Left.Functions[1])
		if limit != 2 {
			require.Len(t, got.Left.Functions, 3)
			require.Same(t, left.Functions[0], got.Left.Functions[2])
			require.Equal(t, &typesv1.FunctionStats{Name: "B"}, got.Right.Functions[2])
		} else {
			require.Len(t, got.Left.Functions, int(limit))
			require.Len(t, got.Right.Functions, int(limit))
		}
		require.True(t, originalLeft.EqualVT(left))
		require.True(t, originalRight.EqualVT(right))
	}
}

func TestDiffValidatesProjectionResponses(t *testing.T) {
	for _, format := range []querierv1.ProfileFormat{querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS} {
		t.Run(format.String(), func(t *testing.T) {
			valid := connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{})
			valid.Msg.Functions = &typesv1.FunctionTable{}
			missing := connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{})
			for _, tc := range []struct {
				name      string
				left      *connect.Response[querierv1.SelectMergeStacktracesResponse]
				right     *connect.Response[querierv1.SelectMergeStacktracesResponse]
				wantError bool
			}{
				{"missing both payloads", missing, missing, true},
				{"missing left payload", missing, valid, true},
				{"missing right payload", valid, missing, true},
				{"nil response", nil, valid, true},
				{"nil message", &connect.Response[querierv1.SelectMergeStacktracesResponse]{}, valid, true},
				{"explicit empty payloads", valid, valid, false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					req := &querierv1.DiffRequest{
						Left: &querierv1.SelectMergeStacktracesRequest{
							Format:        format,
							ProfileTypeID: "cpu",
							LabelSelector: "left",
						},
						Right: &querierv1.SelectMergeStacktracesRequest{
							ProfileTypeID: "cpu",
							LabelSelector: "right",
						},
					}
					resp, err := Diff(context.Background(), req, func(_ context.Context, r *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
						if r.Msg.LabelSelector == "left" {
							return tc.left, nil
						}
						return tc.right, nil
					})
					if tc.wantError {
						require.Equal(t, connect.CodeInternal, connect.CodeOf(err))
						require.Nil(t, resp)
						return
					}
					require.NoError(t, err)
					require.NotNil(t, resp)
				})
			}
		})
	}
}
