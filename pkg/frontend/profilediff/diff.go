package profilediff

import (
	"cmp"
	"context"
	"errors"
	"slices"

	"connectrpc.com/connect"
	"golang.org/x/sync/errgroup"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

type SelectFunc func(context.Context, *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error)

func Diff(ctx context.Context, req *querierv1.DiffRequest, selectProfile SelectFunc) (*connect.Response[querierv1.DiffResponse], error) {
	if req.Left == nil || req.Right == nil || req.Left.ProfileTypeID != req.Right.ProfileTypeID {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("projection diff requires two profiles of the same type"))
	}
	var results [2]*querierv1.SelectMergeStacktracesResponse
	g, ctx := errgroup.WithContext(ctx)
	for i, side := range []*querierv1.SelectMergeStacktracesRequest{req.Left, req.Right} {
		g.Go(func() error {
			q := side.CloneVT()
			q.Format = req.Left.Format
			q.FormatOptions = req.Left.FormatOptions.CloneVT()
			if options := q.GetFormatOptions().GetFunctions(); options != nil && options.Limit > 0 {
				options.Limit = 0
			}

			// Diff owns the two requests and must receive completed projections.
			q.Async = nil
			resp, err := selectProfile(ctx, connect.NewRequest(q))
			if err != nil {
				return err
			}
			if resp == nil || resp.Msg == nil {
				return connect.NewError(connect.CodeInternal, errors.New("profile selection returned no response"))
			}
			results[i] = resp.Msg
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	resp := new(querierv1.DiffResponse)
	switch req.Left.Format {
	case querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTION_TREE:
		if results[0].FunctionTree == nil || results[1].FunctionTree == nil {
			return nil, connect.NewError(connect.CodeInternal, errors.New("profile selection returned no function tree"))
		}
		resp.FunctionTree = &querierv1.FunctionTreeDiff{
			Left:  results[0].FunctionTree,
			Right: results[1].FunctionTree,
		}
	case querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS:
		if results[0].Functions == nil || results[1].Functions == nil {
			return nil, connect.NewError(connect.CodeInternal, errors.New("profile selection returned no function table"))
		}
		resp.Functions = joinFunctions(results[0].Functions, results[1].Functions, req.Left.GetFormatOptions().GetFunctions().GetLimit())
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid projection diff format"))
	}
	return connect.NewResponse(resp), nil
}

// joinFunctions reuses input rows without modifying them; only absent sides need new rows.
func joinFunctions(left, right *typesv1.FunctionTable, limit int64) *querierv1.FunctionTableDiff {
	type pair struct {
		name  string
		left  *typesv1.FunctionStats
		right *typesv1.FunctionStats
	}
	byName := make(map[string]*pair)
	for i, table := range []*typesv1.FunctionTable{left, right} {
		for _, f := range table.GetFunctions() {
			p := byName[f.Name]
			if p == nil {
				p = &pair{name: f.Name}
				byName[f.Name] = p
			}
			if i == 0 {
				p.left = f
			} else {
				p.right = f
			}
		}
	}
	rows := make([]*pair, 0, len(byName))
	for _, p := range byName {
		rows = append(rows, p)
	}
	slices.SortFunc(rows, func(a, b *pair) int {
		if c := cmp.Compare(max(b.left.GetSelf(), b.right.GetSelf()), max(a.left.GetSelf(), a.right.GetSelf())); c != 0 {
			return c
		}
		if c := cmp.Compare(max(b.left.GetTotal(), b.right.GetTotal()), max(a.left.GetTotal(), a.right.GetTotal())); c != 0 {
			return c
		}
		return cmp.Compare(a.name, b.name)
	})
	if limit > 0 && int64(len(rows)) > limit {
		rows = rows[:limit]
	}
	result := &querierv1.FunctionTableDiff{
		Left: &typesv1.FunctionTable{
			Functions:      make([]*typesv1.FunctionStats, 0, len(rows)),
			Total:          left.GetTotal(),
			TotalFunctions: int64(len(byName)),
		},
		Right: &typesv1.FunctionTable{
			Functions:      make([]*typesv1.FunctionStats, 0, len(rows)),
			Total:          right.GetTotal(),
			TotalFunctions: int64(len(byName)),
		},
	}
	for _, row := range rows {
		if row.left == nil {
			row.left = &typesv1.FunctionStats{Name: row.name}
		}
		if row.right == nil {
			row.right = &typesv1.FunctionStats{Name: row.name}
		}
		result.Left.Functions = append(result.Left.Functions, row.left)
		result.Right.Functions = append(result.Right.Functions, row.right)
	}
	return result
}
