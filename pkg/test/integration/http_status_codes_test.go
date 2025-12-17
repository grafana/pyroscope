package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type Request struct {
	ProfileType string
}

func TestStatusCodes(t *testing.T) {
	const profileTypeProcessCPU = "process_cpu:cpu:nanoseconds:cpu:nanoseconds"

	toJSON := func(obj any) string {
		bytes, err := json.Marshal(obj)
		if err != nil {
			panic(fmt.Sprintf("failed to marshal to json: %v", err))
		}
		return string(bytes)
	}

	type Test struct {
		Name             string
		Method           string
		Params           url.Values
		Header           http.Header
		Body             string
		WantV1StatusCode int // For test cases which expect a different v1 status code
	}

	type EndpointTestGroup struct {
		Path string
		// All the test cases partitioned by expected HTTP status code.
		Tests map[int][]Test
	}

	renderTests := EndpointTestGroup{
		Path: "/pyroscope/render",
		Tests: map[int][]Test{
			http.StatusOK: {
				{
					Name:   "valid",
					Method: http.MethodGet,
					Params: url.Values{
						"query": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"from":  []string{"now-15m"},
						"until": []string{"now"},
					},
				},
				{
					Name:   "format_dot_valid",
					Method: http.MethodGet,
					Params: url.Values{
						"query":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"from":   []string{"now-15m"},
						"until":  []string{"now"},
						"format": []string{"dot"},
					},
				},
				{
					Name:   "format_dot_with_max_nodes",
					Method: http.MethodGet,
					Params: url.Values{
						"query":    []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"from":     []string{"now-15m"},
						"until":    []string{"now"},
						"format":   []string{"dot"},
						"maxNodes": []string{"50"},
					},
				},
				{
					Name:   "format_dot_with_max_nodes_hyphen",
					Method: http.MethodGet,
					Params: url.Values{
						"query":     []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"from":      []string{"now-15m"},
						"until":     []string{"now"},
						"format":    []string{"dot"},
						"max-nodes": []string{"50"},
					},
				},
				{
					Name:   "with_group_by",
					Method: http.MethodGet,
					Params: url.Values{
						"query":   []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"from":    []string{"now-15m"},
						"until":   []string{"now"},
						"groupBy": []string{"service_name"},
					},
				},
				{
					Name:   "with_group_by_multiple",
					Method: http.MethodGet,
					Params: url.Values{
						"query":   []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"from":    []string{"now-15m"},
						"until":   []string{"now"},
						"groupBy": []string{"service_name", "region"},
					},
				},
				{
					Name:   "with_aggregation_sum",
					Method: http.MethodGet,
					Params: url.Values{
						"query":       []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"from":        []string{"now-15m"},
						"until":       []string{"now"},
						"aggregation": []string{"sum"},
					},
				},
				{
					Name:   "with_aggregation_avg",
					Method: http.MethodGet,
					Params: url.Values{
						"query":       []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"from":        []string{"now-15m"},
						"until":       []string{"now"},
						"aggregation": []string{"avg"},
					},
				},
				{
					Name:   "with_aggregation_invalid",
					Method: http.MethodGet,
					Params: url.Values{
						"query":       []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"from":        []string{"now-15m"},
						"until":       []string{"now"},
						"aggregation": []string{"invalid"},
					},
				},
				{
					Name:   "with_max_nodes",
					Method: http.MethodGet,
					Params: url.Values{
						"query":    []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"from":     []string{"now-15m"},
						"until":    []string{"now"},
						"maxNodes": []string{"1024"},
					},
				},
				{
					Name:   "with_max_nodes_hyphen",
					Method: http.MethodGet,
					Params: url.Values{
						"query":     []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"from":      []string{"now-15m"},
						"until":     []string{"now"},
						"max-nodes": []string{"1024"},
					},
				},
				{
					Name:   "with_max_nodes_zero",
					Method: http.MethodGet,
					Params: url.Values{
						"query":    []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"from":     []string{"now-15m"},
						"until":    []string{"now"},
						"maxNodes": []string{"0"},
					},
				},
				{
					Name:   "with_max_nodes_invalid",
					Method: http.MethodGet,
					Params: url.Values{
						"query":    []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"from":     []string{"now-15m"},
						"until":    []string{"now"},
						"maxNodes": []string{"invalid"},
					},
				},
				{
					Name:   "all_params_combined",
					Method: http.MethodGet,
					Params: url.Values{
						"query":       []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"from":        []string{"now-15m"},
						"until":       []string{"now"},
						"groupBy":     []string{"service_name"},
						"aggregation": []string{"sum"},
						"maxNodes":    []string{"512"},
					},
				},
			},
			http.StatusBadRequest: {
				{
					Name:   "no_time_range",
					Method: http.MethodGet,
					Params: url.Values{
						"query": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
					},
				},
				{
					Name:   "all_zero_time_range",
					Method: http.MethodGet,
					Params: url.Values{
						"query": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"from":  []string{"0"},
						"until": []string{"0"},
					},
				},
				{
					Name:   "from_zero_time_range",
					Method: http.MethodGet,
					Params: url.Values{
						"query": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"from":  []string{"0"},
						"until": []string{"now"},
					},
				},
				{
					Name:   "until_zero_time_range",
					Method: http.MethodGet,
					Params: url.Values{
						"query": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"from":  []string{"now-15m"},
						"until": []string{"0"},
					},
				},
				{
					Name:   "missing_query",
					Method: http.MethodGet,
					Params: url.Values{
						"from":  []string{"now-15m"},
						"until": []string{"now"},
					},
				},
				{
					Name:   "invalid_query_syntax",
					Method: http.MethodGet,
					Params: url.Values{
						"query": []string{"bad_query"},
						"from":  []string{"now-15m"},
						"until": []string{"now"},
					},
				},
				{
					Name:   "query_without_profile_type",
					Method: http.MethodGet,
					Params: url.Values{
						"query": []string{`{service_name="test"}`},
						"from":  []string{"now-15m"},
						"until": []string{"now"},
					},
				},
				{
					Name:   "invalid_profile_type_format",
					Method: http.MethodGet,
					Params: url.Values{
						"query": []string{`invalid_format{service_name="test"}`},
						"from":  []string{"now-15m"},
						"until": []string{"now"},
					},
				},
				{
					Name:   "format_dot_no_time_range",
					Method: http.MethodGet,
					Params: url.Values{
						"query":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"format": []string{"dot"},
					},
				},
				{
					Name:   "empty_query_param",
					Method: http.MethodGet,
					Params: url.Values{
						"query": []string{""},
						"from":  []string{"now-15m"},
						"until": []string{"now"},
					},
				},
				{
					Name:   "from_and_until_0_with_dot_format",
					Method: http.MethodGet,
					Params: url.Values{
						"query":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"from":   []string{"0"},
						"until":  []string{"0"},
						"format": []string{"dot"},
					},
				},
			},
			http.StatusMethodNotAllowed: {
				{
					Name:   "post_method_not_allowed",
					Method: http.MethodPost,
					Params: url.Values{
						"query": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"from":  []string{"now-15m"},
						"until": []string{"now"},
					},
				},
				{
					Name:   "put_method_not_allowed",
					Method: http.MethodPut,
					Params: url.Values{
						"query": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"from":  []string{"now-15m"},
						"until": []string{"now"},
					},
				},
				{
					Name:   "delete_method_not_allowed",
					Method: http.MethodDelete,
					Params: url.Values{
						"query": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"from":  []string{"now-15m"},
						"until": []string{"now"},
					},
				},
				{
					Name:   "patch_method_not_allowed",
					Method: http.MethodPatch,
					Params: url.Values{
						"query": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"from":  []string{"now-15m"},
						"until": []string{"now"},
					},
				},
			},
		},
	}

	renderDiffTests := EndpointTestGroup{
		Path: "/pyroscope/render-diff",
		Tests: map[int][]Test{
			http.StatusOK: {
				{
					Name:   "valid",
					Method: http.MethodGet,
					Params: url.Values{
						"leftQuery":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"leftFrom":   []string{"now-30m"},
						"leftUntil":  []string{"now-15m"},
						"rightQuery": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"rightFrom":  []string{"now-15m"},
						"rightUntil": []string{"now"},
					},
				},
				{
					Name:   "same_query_different_times",
					Method: http.MethodGet,
					Params: url.Values{
						"leftQuery":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"leftFrom":   []string{"now-60m"},
						"leftUntil":  []string{"now-30m"},
						"rightQuery": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"rightFrom":  []string{"now-30m"},
						"rightUntil": []string{"now"},
					},
				},
				{
					Name:   "same_time_range",
					Method: http.MethodGet,
					Params: url.Values{
						"leftQuery":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"leftFrom":   []string{"now-15m"},
						"leftUntil":  []string{"now"},
						"rightQuery": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"rightFrom":  []string{"now-15m"},
						"rightUntil": []string{"now"},
					},
				},
			},
			http.StatusBadRequest: {
				{
					Name:   "missing_left_query",
					Method: http.MethodGet,
					Params: url.Values{
						"leftFrom":   []string{"now-30m"},
						"leftUntil":  []string{"now-15m"},
						"rightQuery": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"rightFrom":  []string{"now-15m"},
						"rightUntil": []string{"now"},
					},
				},
				{
					Name:   "missing_right_query",
					Method: http.MethodGet,
					Params: url.Values{
						"leftQuery":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"leftFrom":   []string{"now-30m"},
						"leftUntil":  []string{"now-15m"},
						"rightFrom":  []string{"now-15m"},
						"rightUntil": []string{"now"},
					},
				},
				{
					Name:   "missing_left_from",
					Method: http.MethodGet,
					Params: url.Values{
						"leftQuery":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"leftUntil":  []string{"now-15m"},
						"rightQuery": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"rightFrom":  []string{"now-15m"},
						"rightUntil": []string{"now"},
					},
				},
				{
					Name:   "missing_left_until",
					Method: http.MethodGet,
					Params: url.Values{
						"leftQuery":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"leftFrom":   []string{"now-30m"},
						"rightQuery": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"rightFrom":  []string{"now-15m"},
						"rightUntil": []string{"now"},
					},
				},
				{
					Name:   "missing_right_from",
					Method: http.MethodGet,
					Params: url.Values{
						"leftQuery":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"leftFrom":   []string{"now-30m"},
						"leftUntil":  []string{"now-15m"},
						"rightQuery": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"rightUntil": []string{"now"},
					},
				},
				{
					Name:   "missing_right_until",
					Method: http.MethodGet,
					Params: url.Values{
						"leftQuery":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"leftFrom":   []string{"now-30m"},
						"leftUntil":  []string{"now-15m"},
						"rightQuery": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"rightFrom":  []string{"now-15m"},
					},
				},
				{
					Name:   "invalid_left_query_syntax",
					Method: http.MethodGet,
					Params: url.Values{
						"leftQuery":  []string{"bad_query"},
						"leftFrom":   []string{"now-30m"},
						"leftUntil":  []string{"now-15m"},
						"rightQuery": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"rightFrom":  []string{"now-15m"},
						"rightUntil": []string{"now"},
					},
				},
				{
					Name:   "invalid_right_query_syntax",
					Method: http.MethodGet,
					Params: url.Values{
						"leftQuery":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"leftFrom":   []string{"now-30m"},
						"leftUntil":  []string{"now-15m"},
						"rightQuery": []string{"bad_query"},
						"rightFrom":  []string{"now-15m"},
						"rightUntil": []string{"now"},
					},
				},
				{
					Name:   "left_query_without_profile_type",
					Method: http.MethodGet,
					Params: url.Values{
						"leftQuery":  []string{`{service_name="test"}`},
						"leftFrom":   []string{"now-30m"},
						"leftUntil":  []string{"now-15m"},
						"rightQuery": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"rightFrom":  []string{"now-15m"},
						"rightUntil": []string{"now"},
					},
				},
				{
					Name:   "right_query_without_profile_type",
					Method: http.MethodGet,
					Params: url.Values{
						"leftQuery":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"leftFrom":   []string{"now-30m"},
						"leftUntil":  []string{"now-15m"},
						"rightQuery": []string{`{service_name="test"}`},
						"rightFrom":  []string{"now-15m"},
						"rightUntil": []string{"now"},
					},
				},
				{
					Name:   "invalid_left_profile_type_format",
					Method: http.MethodGet,
					Params: url.Values{
						"leftQuery":  []string{`invalid_format{service_name="test"}`},
						"leftFrom":   []string{"now-30m"},
						"leftUntil":  []string{"now-15m"},
						"rightQuery": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"rightFrom":  []string{"now-15m"},
						"rightUntil": []string{"now"},
					},
				},
				{
					Name:   "invalid_right_profile_type_format",
					Method: http.MethodGet,
					Params: url.Values{
						"leftQuery":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"leftFrom":   []string{"now-30m"},
						"leftUntil":  []string{"now-15m"},
						"rightQuery": []string{`invalid_format{service_name="test"}`},
						"rightFrom":  []string{"now-15m"},
						"rightUntil": []string{"now"},
					},
				},
				{
					Name:   "profile_types_mismatch",
					Method: http.MethodGet,
					Params: url.Values{
						"leftQuery":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"leftFrom":   []string{"now-30m"},
						"leftUntil":  []string{"now-15m"},
						"rightQuery": []string{createRenderQuery("memory:alloc_objects:count:space:bytes", "pyroscope")},
						"rightFrom":  []string{"now-15m"},
						"rightUntil": []string{"now"},
					},
				},
				{
					Name:   "empty_left_query",
					Method: http.MethodGet,
					Params: url.Values{
						"leftQuery":  []string{""},
						"leftFrom":   []string{"now-30m"},
						"leftUntil":  []string{"now-15m"},
						"rightQuery": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"rightFrom":  []string{"now-15m"},
						"rightUntil": []string{"now"},
					},
				},
				{
					Name:   "empty_right_query",
					Method: http.MethodGet,
					Params: url.Values{
						"leftQuery":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"leftFrom":   []string{"now-30m"},
						"leftUntil":  []string{"now-15m"},
						"rightQuery": []string{""},
						"rightFrom":  []string{"now-15m"},
						"rightUntil": []string{"now"},
					},
				},
				{
					Name:   "left_from_zero",
					Method: http.MethodGet,
					Params: url.Values{
						"leftQuery":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"leftFrom":   []string{"0"},
						"leftUntil":  []string{"now-15m"},
						"rightQuery": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"rightFrom":  []string{"now-15m"},
						"rightUntil": []string{"now"},
					},
				},
				{
					Name:   "left_until_zero",
					Method: http.MethodGet,
					Params: url.Values{
						"leftQuery":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"leftFrom":   []string{"now-30m"},
						"leftUntil":  []string{"0"},
						"rightQuery": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"rightFrom":  []string{"now-15m"},
						"rightUntil": []string{"now"},
					},
				},
				{
					Name:   "right_from_zero",
					Method: http.MethodGet,
					Params: url.Values{
						"leftQuery":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"leftFrom":   []string{"now-30m"},
						"leftUntil":  []string{"now-15m"},
						"rightQuery": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"rightFrom":  []string{"0"},
						"rightUntil": []string{"now"},
					},
				},
				{
					Name:   "right_until_zero",
					Method: http.MethodGet,
					Params: url.Values{
						"leftQuery":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"leftFrom":   []string{"now-30m"},
						"leftUntil":  []string{"now-15m"},
						"rightQuery": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"rightFrom":  []string{"now-15m"},
						"rightUntil": []string{"0"},
					},
				},
				{
					Name:   "all_zero_time_ranges",
					Method: http.MethodGet,
					Params: url.Values{
						"leftQuery":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"leftFrom":   []string{"0"},
						"leftUntil":  []string{"0"},
						"rightQuery": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"rightFrom":  []string{"0"},
						"rightUntil": []string{"0"},
					},
				},
			},
			http.StatusMethodNotAllowed: {
				{
					Name:   "post_method_not_allowed",
					Method: http.MethodPost,
					Params: url.Values{
						"leftQuery":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"leftFrom":   []string{"now-30m"},
						"leftUntil":  []string{"now-15m"},
						"rightQuery": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"rightFrom":  []string{"now-15m"},
						"rightUntil": []string{"now"},
					},
				},
				{
					Name:   "put_method_not_allowed",
					Method: http.MethodPut,
					Params: url.Values{
						"leftQuery":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"leftFrom":   []string{"now-30m"},
						"leftUntil":  []string{"now-15m"},
						"rightQuery": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"rightFrom":  []string{"now-15m"},
						"rightUntil": []string{"now"},
					},
				},
				{
					Name:   "delete_method_not_allowed",
					Method: http.MethodDelete,
					Params: url.Values{
						"leftQuery":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"leftFrom":   []string{"now-30m"},
						"leftUntil":  []string{"now-15m"},
						"rightQuery": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"rightFrom":  []string{"now-15m"},
						"rightUntil": []string{"now"},
					},
				},
				{
					Name:   "patch_method_not_allowed",
					Method: http.MethodPatch,
					Params: url.Values{
						"leftQuery":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"leftFrom":   []string{"now-30m"},
						"leftUntil":  []string{"now-15m"},
						"rightQuery": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
						"rightFrom":  []string{"now-15m"},
						"rightUntil": []string{"now"},
					},
				},
			},
		},
	}

	profileTypesTests := EndpointTestGroup{
		Path: "/querier.v1.QuerierService/ProfileTypes",
		Tests: map[int][]Test{
			http.StatusOK: {
				{
					Name:   "valid",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"start": time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":   time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "no_time_range",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{}),
				},
				{
					Name:   "zero_time_range",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"start": 0,
						"end":   0,
					}),
				},
				{
					Name:   "valid_long_range",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"start": time.Now().Add(-24 * time.Hour).UnixMilli(),
						"end":   time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "valid_short_range",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"start": time.Now().Add(-5 * time.Minute).UnixMilli(),
						"end":   time.Now().UnixMilli(),
					}),
				},
				{
					// TODO(bryan) this should be fixed
					Name:   "negative_start",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"start": -1,
						"end":   time.Now().UnixMilli(),
					}),
				},
				{
					// TODO(bryan) this should be fixed
					Name:   "negative_end",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"start": -2,
						"end":   -1,
					}),
				},
			},
			http.StatusBadRequest: {
				{
					Name:   "invalid_json",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: `{"invalid json"`,
				},
				{
					Name:   "empty_body",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: "",
				},
				{
					Name:   "start_after_end",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"start": time.Now().UnixMilli(),
						"end":   time.Now().Add(-1 * time.Hour).UnixMilli(),
					}),
				},
				{
					Name:   "start_string_instead_of_number",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: `{"start": "now-1h", "end": 0}`,
				},
				{
					Name:   "end_string_instead_of_number",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: `{"start": 0, "end": "now"}`,
				},
			},
			http.StatusMethodNotAllowed: {
				{
					Name:   "get_method_not_allowed",
					Method: http.MethodGet,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Params: url.Values{
						"start": []string{fmt.Sprintf("%d", time.Now().Add(-1*time.Hour).UnixMilli())},
						"end":   []string{fmt.Sprintf("%d", time.Now().UnixMilli())},
					},
				},
				{
					Name:   "put_method_not_allowed",
					Method: http.MethodPut,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"start": time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":   time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "delete_method_not_allowed",
					Method: http.MethodDelete,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"start": time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":   time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "patch_method_not_allowed",
					Method: http.MethodPatch,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"start": time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":   time.Now().UnixMilli(),
					}),
				},
			},
			http.StatusUnsupportedMediaType: {
				{
					Name:   "invalid_content_type",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"text/plain"},
					},
					Body: toJSON(map[string]any{
						"start": time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":   time.Now().UnixMilli(),
					}),
				},
			},
		},
	}

	labelValuesTests := EndpointTestGroup{
		Path: "/querier.v1.QuerierService/LabelValues",
		Tests: map[int][]Test{
			http.StatusOK: {
				{
					Name:   "valid",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"name":  "service_name",
						"start": time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":   time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "valid_with_matchers",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"name":     "service_name",
						"matchers": []string{`{namespace="default"}`},
						"start":    time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":      time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "valid_with_multiple_matchers",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"name":     "service_name",
						"matchers": []string{`{namespace="default"}`, `{region="us-west"}`},
						"start":    time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":      time.Now().UnixMilli(),
					}),
				},
			},
			http.StatusBadRequest: {
				{
					Name:   "invalid_no_time_range",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"name": "service_name",
					}),
					// V1 allows no time ranges
					WantV1StatusCode: http.StatusOK,
				},
				{
					Name:   "invalid_matchers_syntax",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"name":     "service_name",
						"matchers": []string{"!bad_syntax!"},
						"start":    time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":      time.Now().UnixMilli(),
					}),
					WantV1StatusCode: http.StatusOK,
				},
				{
					Name:   "invalid_json",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: `{"invalid json"`,
				},
				{
					Name:   "empty_body",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: "",
				},
				{
					Name:   "start_after_end",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"name":  "service_name",
						"start": time.Now().UnixMilli(),
						"end":   time.Now().Add(-1 * time.Hour).UnixMilli(),
					}),
				},
				{
					Name:   "empty_name",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"name":  "",
						"start": time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":   time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "missing_name",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"start": time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":   time.Now().UnixMilli(),
					}),
				},
			},
			http.StatusMethodNotAllowed: {
				{
					Name:   "get_method_not_allowed",
					Method: http.MethodGet,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
				},
				{
					Name:   "put_method_not_allowed",
					Method: http.MethodPut,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"name":  "service_name",
						"start": time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":   time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "delete_method_not_allowed",
					Method: http.MethodDelete,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"name":  "service_name",
						"start": time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":   time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "patch_method_not_allowed",
					Method: http.MethodPatch,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"name":  "service_name",
						"start": time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":   time.Now().UnixMilli(),
					}),
				},
			},
			http.StatusUnsupportedMediaType: {
				{
					Name:   "invalid_content_type",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"text/plain"},
					},
					Body: toJSON(map[string]any{
						"name":  "service_name",
						"start": time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":   time.Now().UnixMilli(),
					}),
				},
			},
		},
	}

	labelNamesTests := EndpointTestGroup{
		Path: "/querier.v1.QuerierService/LabelNames",
		Tests: map[int][]Test{
			http.StatusOK: {
				{
					Name:   "valid",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"start": time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":   time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "valid_with_matchers",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"matchers": []string{`{namespace="default"}`},
						"start":    time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":      time.Now().UnixMilli(),
					}),
				},
			},
			http.StatusBadRequest: {
				{
					Name:   "valid_no_time_range",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{}),
					// V1 allows no time ranges
					WantV1StatusCode: http.StatusOK,
				},
				{
					Name:   "invalid_json",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: `{"invalid json"`,
				},
				{
					Name:   "empty_body",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: "",
				},
				{
					Name:   "start_after_end",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"start": time.Now().UnixMilli(),
						"end":   time.Now().Add(-1 * time.Hour).UnixMilli(),
					}),
				},
				{
					Name:   "invalid_matchers_syntax",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"matchers": []string{"!bad_syntax!"},
						"start":    time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":      time.Now().UnixMilli(),
					}),
					WantV1StatusCode: http.StatusOK,
				},
			},
			http.StatusMethodNotAllowed: {
				{
					Name:   "get_method_not_allowed",
					Method: http.MethodGet,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
				},
				{
					Name:   "put_method_not_allowed",
					Method: http.MethodPut,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"start": time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":   time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "delete_method_not_allowed",
					Method: http.MethodDelete,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"start": time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":   time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "patch_method_not_allowed",
					Method: http.MethodPatch,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"start": time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":   time.Now().UnixMilli(),
					}),
				},
			},
			http.StatusUnsupportedMediaType: {
				{
					Name:   "invalid_content_type",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"text/plain"},
					},
					Body: toJSON(map[string]any{
						"start": time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":   time.Now().UnixMilli(),
					}),
				},
			},
		},
	}

	seriesTests := EndpointTestGroup{
		Path: "/querier.v1.QuerierService/Series",
		Tests: map[int][]Test{
			http.StatusOK: {
				{
					Name:   "valid",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"matchers": []string{`{service_name="test"}`},
						"start":    time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":      time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "valid_with_label_names",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"matchers":   []string{`{service_name="test"}`},
						"labelNames": []string{"service_name", "namespace"},
						"start":      time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":        time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "valid_no_matchers",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"start": time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":   time.Now().UnixMilli(),
					}),
				},
			},
			http.StatusBadRequest: {
				{
					Name:   "invalid_json",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: `{"invalid json"`,
				},
				{
					Name:   "empty_body",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: "",
				},
				{
					Name:   "start_after_end",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"matchers": []string{`{service_name="test"}`},
						"start":    time.Now().UnixMilli(),
						"end":      time.Now().Add(-1 * time.Hour).UnixMilli(),
					}),
				},
				{
					Name:   "invalid_no_time_range",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"matchers": []string{`{service_name="test"}`},
					}),
					// V1 allows no time ranges
					WantV1StatusCode: http.StatusOK,
				},
				{
					Name:   "invalid_matchers_syntax",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"matchers": []string{"!bad_syntax!"},
						"start":    time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":      time.Now().UnixMilli(),
					}),
					WantV1StatusCode: http.StatusOK,
				},
			},
			http.StatusMethodNotAllowed: {
				{
					Name:   "get_method_not_allowed",
					Method: http.MethodGet,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
				},
				{
					Name:   "put_method_not_allowed",
					Method: http.MethodPut,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"matchers": []string{`{service_name="test"}`},
						"start":    time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":      time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "delete_method_not_allowed",
					Method: http.MethodDelete,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"matchers": []string{`{service_name="test"}`},
						"start":    time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":      time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "patch_method_not_allowed",
					Method: http.MethodPatch,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"matchers": []string{`{service_name="test"}`},
						"start":    time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":      time.Now().UnixMilli(),
					}),
				},
			},
			http.StatusUnsupportedMediaType: {
				{
					Name:   "invalid_content_type",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"text/plain"},
					},
					Body: toJSON(map[string]any{
						"matchers": []string{`{service_name="test"}`},
						"start":    time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":      time.Now().UnixMilli(),
					}),
				},
			},
		},
	}

	selectMergeStacktracesTests := EndpointTestGroup{
		Path: "/querier.v1.QuerierService/SelectMergeStacktraces",
		Tests: map[int][]Test{
			http.StatusOK: {
				{
					Name:   "valid",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "valid_with_max_nodes",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"maxNodes":      1024,
					}),
				},
				{
					Name:   "valid_with_format_tree",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"format":        2,
					}),
				},
				{
					Name:   "valid_with_label_selector",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": `{service_name="test"}`,
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
					}),
				},
			},
			http.StatusBadRequest: {
				{
					Name:   "missing_profile_type_id",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "empty_profile_type_id",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": "",
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "invalid_profile_type_format",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": "invalid_format",
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "missing_label_selector",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "invalid_label_selector",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "!bad_syntax!",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "invalid_json",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: `{"invalid json"`,
				},
				{
					Name:   "empty_body",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: "",
				},
				{
					Name:   "start_after_end",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().UnixMilli(),
						"end":           time.Now().Add(-1 * time.Hour).UnixMilli(),
					}),
				},
				{
					Name:   "invalid_no_time_range",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
					}),
				},
			},
			http.StatusMethodNotAllowed: {
				{
					Name:   "get_method_not_allowed",
					Method: http.MethodGet,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
				},
				{
					Name:   "put_method_not_allowed",
					Method: http.MethodPut,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "delete_method_not_allowed",
					Method: http.MethodDelete,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "patch_method_not_allowed",
					Method: http.MethodPatch,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
					}),
				},
			},
			http.StatusUnsupportedMediaType: {
				{
					Name:   "invalid_content_type",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"text/plain"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
					}),
				},
			},
		},
	}

	selectMergeProfileTests := EndpointTestGroup{
		Path: "/querier.v1.QuerierService/SelectMergeProfile",
		Tests: map[int][]Test{
			http.StatusOK: {
				{
					Name:   "valid",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "valid_with_max_nodes",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"maxNodes":      1024,
					}),
				},
				{
					Name:   "valid_with_label_selector",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": `{service_name="test"}`,
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
					}),
				},
			},
			http.StatusBadRequest: {
				{
					Name:   "missing_profile_type_id",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "empty_profile_type_id",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": "",
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "invalid_profile_type_format",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": "invalid_format",
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "missing_label_selector",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "invalid_label_selector",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "!bad_syntax!",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "invalid_json",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: `{"invalid json"`,
				},
				{
					Name:   "empty_body",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: "",
				},
				{
					Name:   "start_after_end",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().UnixMilli(),
						"end":           time.Now().Add(-1 * time.Hour).UnixMilli(),
					}),
				},
				{
					Name:   "valid_no_time_range",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
					}),
				},
			},
			http.StatusMethodNotAllowed: {
				{
					Name:   "get_method_not_allowed",
					Method: http.MethodGet,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
				},
				{
					Name:   "put_method_not_allowed",
					Method: http.MethodPut,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "delete_method_not_allowed",
					Method: http.MethodDelete,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
					}),
				},
				{
					Name:   "patch_method_not_allowed",
					Method: http.MethodPatch,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
					}),
				},
			},
			http.StatusUnsupportedMediaType: {
				{
					Name:   "invalid_content_type",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"text/plain"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
					}),
				},
			},
		},
	}

	selectSeriesTests := EndpointTestGroup{
		Path: "/querier.v1.QuerierService/SelectSeries",
		Tests: map[int][]Test{
			http.StatusOK: {
				{
					Name:   "valid",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"step":          15.0,
					}),
				},
				{
					Name:   "valid_with_group_by",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"step":          15.0,
						"groupBy":       []string{"service_name"},
					}),
				},
				{
					Name:   "valid_with_limit",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"step":          15.0,
						"limit":         10,
					}),
				},
			},
			http.StatusBadRequest: {
				{
					Name:   "missing_profile_type_id",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"step":          15.0,
					}),
				},
				{
					Name:   "empty_profile_type_id",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": "",
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"step":          15.0,
					}),
				},
				{
					Name:   "invalid_profile_type_format",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": "invalid_format",
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"step":          15.0,
					}),
				},
				{
					Name:   "missing_label_selector",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"step":          15.0,
					}),
				},
				{
					Name:   "invalid_label_selector",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "!bad_syntax!",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"step":          15.0,
					}),
				},
				{
					Name:   "invalid_no_step",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
					}),
					WantV1StatusCode: http.StatusBadRequest,
				},
				{
					Name:   "invalid_json",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: `{"invalid json"`,
				},
				{
					Name:   "empty_body",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: "",
				},
				{
					Name:   "start_after_end",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().UnixMilli(),
						"end":           time.Now().Add(-1 * time.Hour).UnixMilli(),
						"step":          15.0,
					}),
				},
				{
					Name:   "invalid_no_time_range",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"step":          15.0,
					}),
				},
			},
			http.StatusMethodNotAllowed: {
				{
					Name:   "get_method_not_allowed",
					Method: http.MethodGet,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
				},
				{
					Name:   "put_method_not_allowed",
					Method: http.MethodPut,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"step":          15.0,
					}),
				},
				{
					Name:   "delete_method_not_allowed",
					Method: http.MethodDelete,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"step":          15.0,
					}),
				},
				{
					Name:   "patch_method_not_allowed",
					Method: http.MethodPatch,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"step":          15.0,
					}),
				},
			},
			http.StatusUnsupportedMediaType: {
				{
					Name:   "invalid_content_type",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"text/plain"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"step":          15.0,
					}),
				},
			},
		},
	}

	diffTests := EndpointTestGroup{
		Path: "/querier.v1.QuerierService/Diff",
		Tests: map[int][]Test{
			http.StatusOK: {
				{
					Name:   "valid",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"left": map[string]any{
							"profileTypeID": profileTypeProcessCPU,
							"labelSelector": "{}",
							"start":         time.Now().Add(-2 * time.Hour).UnixMilli(),
							"end":           time.Now().Add(-1 * time.Hour).UnixMilli(),
						},
						"right": map[string]any{
							"profileTypeID": profileTypeProcessCPU,
							"labelSelector": "{}",
							"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
							"end":           time.Now().UnixMilli(),
						},
					}),
				},
				{
					Name:   "valid_with_max_nodes",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"left": map[string]any{
							"profileTypeID": profileTypeProcessCPU,
							"labelSelector": "{}",
							"start":         time.Now().Add(-2 * time.Hour).UnixMilli(),
							"end":           time.Now().Add(-1 * time.Hour).UnixMilli(),
							"maxNodes":      1024,
						},
						"right": map[string]any{
							"profileTypeID": profileTypeProcessCPU,
							"labelSelector": "{}",
							"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
							"end":           time.Now().UnixMilli(),
							"maxNodes":      512,
						},
					}),
				},
				{
					Name:   "valid_same_time_range",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"left": map[string]any{
							"profileTypeID": profileTypeProcessCPU,
							"labelSelector": "{}",
							"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
							"end":           time.Now().UnixMilli(),
						},
						"right": map[string]any{
							"profileTypeID": profileTypeProcessCPU,
							"labelSelector": "{}",
							"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
							"end":           time.Now().UnixMilli(),
						},
					}),
				},
			},
			http.StatusBadRequest: {
				{
					Name:   "missing_left",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"right": map[string]any{
							"profileTypeID": profileTypeProcessCPU,
							"labelSelector": "{}",
							"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
							"end":           time.Now().UnixMilli(),
						},
					}),
				},
				{
					Name:   "missing_right",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"left": map[string]any{
							"profileTypeID": profileTypeProcessCPU,
							"labelSelector": "{}",
							"start":         time.Now().Add(-2 * time.Hour).UnixMilli(),
							"end":           time.Now().Add(-1 * time.Hour).UnixMilli(),
						},
					}),
				},
				{
					Name:   "left_missing_profile_type_id",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"left": map[string]any{
							"labelSelector": "{}",
							"start":         time.Now().Add(-2 * time.Hour).UnixMilli(),
							"end":           time.Now().Add(-1 * time.Hour).UnixMilli(),
						},
						"right": map[string]any{
							"profileTypeID": profileTypeProcessCPU,
							"labelSelector": "{}",
							"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
							"end":           time.Now().UnixMilli(),
						},
					}),
				},
				{
					Name:   "right_missing_profile_type_id",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"left": map[string]any{
							"profileTypeID": profileTypeProcessCPU,
							"labelSelector": "{}",
							"start":         time.Now().Add(-2 * time.Hour).UnixMilli(),
							"end":           time.Now().Add(-1 * time.Hour).UnixMilli(),
						},
						"right": map[string]any{
							"labelSelector": "{}",
							"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
							"end":           time.Now().UnixMilli(),
						},
					}),
				},
				{
					Name:   "left_invalid_profile_type",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"left": map[string]any{
							"profileTypeID": "invalid_format",
							"labelSelector": "{}",
							"start":         time.Now().Add(-2 * time.Hour).UnixMilli(),
							"end":           time.Now().Add(-1 * time.Hour).UnixMilli(),
						},
						"right": map[string]any{
							"profileTypeID": profileTypeProcessCPU,
							"labelSelector": "{}",
							"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
							"end":           time.Now().UnixMilli(),
						},
					}),
				},
				{
					Name:   "right_invalid_profile_type",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"left": map[string]any{
							"profileTypeID": profileTypeProcessCPU,
							"labelSelector": "{}",
							"start":         time.Now().Add(-2 * time.Hour).UnixMilli(),
							"end":           time.Now().Add(-1 * time.Hour).UnixMilli(),
						},
						"right": map[string]any{
							"profileTypeID": "invalid_format",
							"labelSelector": "{}",
							"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
							"end":           time.Now().UnixMilli(),
						},
					}),
				},
				{
					Name:   "left_missing_label_selector",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"left": map[string]any{
							"profileTypeID": profileTypeProcessCPU,
							"start":         time.Now().Add(-2 * time.Hour).UnixMilli(),
							"end":           time.Now().Add(-1 * time.Hour).UnixMilli(),
						},
						"right": map[string]any{
							"profileTypeID": profileTypeProcessCPU,
							"labelSelector": "{}",
							"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
							"end":           time.Now().UnixMilli(),
						},
					}),
				},
				{
					Name:   "right_missing_label_selector",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"left": map[string]any{
							"profileTypeID": profileTypeProcessCPU,
							"labelSelector": "{}",
							"start":         time.Now().Add(-2 * time.Hour).UnixMilli(),
							"end":           time.Now().Add(-1 * time.Hour).UnixMilli(),
						},
						"right": map[string]any{
							"profileTypeID": profileTypeProcessCPU,
							"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
							"end":           time.Now().UnixMilli(),
						},
					}),
				},
				{
					Name:   "invalid_json",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: `{"invalid json"`,
				},
				{
					Name:   "empty_body",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: "",
				},
			},
			http.StatusMethodNotAllowed: {
				{
					Name:   "get_method_not_allowed",
					Method: http.MethodGet,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
				},
				{
					Name:   "put_method_not_allowed",
					Method: http.MethodPut,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"left": map[string]any{
							"profileTypeID": profileTypeProcessCPU,
							"labelSelector": "{}",
							"start":         time.Now().Add(-2 * time.Hour).UnixMilli(),
							"end":           time.Now().Add(-1 * time.Hour).UnixMilli(),
						},
						"right": map[string]any{
							"profileTypeID": profileTypeProcessCPU,
							"labelSelector": "{}",
							"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
							"end":           time.Now().UnixMilli(),
						},
					}),
				},
				{
					Name:   "delete_method_not_allowed",
					Method: http.MethodDelete,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"left": map[string]any{
							"profileTypeID": profileTypeProcessCPU,
							"labelSelector": "{}",
							"start":         time.Now().Add(-2 * time.Hour).UnixMilli(),
							"end":           time.Now().Add(-1 * time.Hour).UnixMilli(),
						},
						"right": map[string]any{
							"profileTypeID": profileTypeProcessCPU,
							"labelSelector": "{}",
							"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
							"end":           time.Now().UnixMilli(),
						},
					}),
				},
				{
					Name:   "patch_method_not_allowed",
					Method: http.MethodPatch,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"left": map[string]any{
							"profileTypeID": profileTypeProcessCPU,
							"labelSelector": "{}",
							"start":         time.Now().Add(-2 * time.Hour).UnixMilli(),
							"end":           time.Now().Add(-1 * time.Hour).UnixMilli(),
						},
						"right": map[string]any{
							"profileTypeID": profileTypeProcessCPU,
							"labelSelector": "{}",
							"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
							"end":           time.Now().UnixMilli(),
						},
					}),
				},
			},
			http.StatusUnsupportedMediaType: {
				{
					Name:   "invalid_content_type",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"text/plain"},
					},
					Body: toJSON(map[string]any{
						"left": map[string]any{
							"profileTypeID": profileTypeProcessCPU,
							"labelSelector": "{}",
							"start":         time.Now().Add(-2 * time.Hour).UnixMilli(),
							"end":           time.Now().Add(-1 * time.Hour).UnixMilli(),
						},
						"right": map[string]any{
							"profileTypeID": profileTypeProcessCPU,
							"labelSelector": "{}",
							"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
							"end":           time.Now().UnixMilli(),
						},
					}),
				},
			},
		},
	}

	selectMergeSpanProfileTests := EndpointTestGroup{
		Path: "/querier.v1.QuerierService/SelectMergeSpanProfile",
		Tests: map[int][]Test{
			http.StatusOK: {
				{
					Name:   "valid",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"spanSelector":  []string{"span1", "span2"},
					}),
				},
				{
					Name:   "valid_with_max_nodes",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"spanSelector":  []string{"span1"},
						"maxNodes":      1024,
					}),
				},
				{
					Name:   "valid_with_format_tree",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"spanSelector":  []string{"span1"},
						"format":        2,
					}),
				},
			},
			http.StatusBadRequest: {
				{
					Name:   "missing_profile_type_id",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"spanSelector":  []string{"span1"},
					}),
				},
				{
					Name:   "empty_profile_type_id",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": "",
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"spanSelector":  []string{"span1"},
					}),
				},
				{
					Name:   "invalid_profile_type_format",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": "invalid_format",
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"spanSelector":  []string{"span1"},
					}),
				},
				{
					Name:   "missing_label_selector",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"spanSelector":  []string{"span1"},
					}),
				},
				{
					Name:   "invalid_label_selector",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "!bad_syntax!",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"spanSelector":  []string{"span1"},
					}),
				},
				{
					Name:   "invalid_json",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: `{"invalid json"`,
				},
				{
					Name:   "empty_body",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: "",
				},
				{
					Name:   "start_after_end",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().UnixMilli(),
						"end":           time.Now().Add(-1 * time.Hour).UnixMilli(),
						"spanSelector":  []string{"span1"},
					}),
				},
				{
					Name:   "invalid_no_time_range",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"spanSelector":  []string{"span1"},
					}),
				},
			},
			http.StatusMethodNotAllowed: {
				{
					Name:   "get_method_not_allowed",
					Method: http.MethodGet,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
				},
				{
					Name:   "put_method_not_allowed",
					Method: http.MethodPut,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"spanSelector":  []string{"span1"},
					}),
				},
				{
					Name:   "delete_method_not_allowed",
					Method: http.MethodDelete,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"spanSelector":  []string{"span1"},
					}),
				},
				{
					Name:   "patch_method_not_allowed",
					Method: http.MethodPatch,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"spanSelector":  []string{"span1"},
					}),
				},
			},
			http.StatusUnsupportedMediaType: {
				{
					Name:   "invalid_content_type",
					Method: http.MethodPost,
					Header: http.Header{
						"Content-Type": []string{"text/plain"},
					},
					Body: toJSON(map[string]any{
						"profileTypeID": profileTypeProcessCPU,
						"labelSelector": "{}",
						"start":         time.Now().Add(-1 * time.Hour).UnixMilli(),
						"end":           time.Now().UnixMilli(),
						"spanSelector":  []string{"span1"},
					}),
				},
			},
		},
	}

	allTests := []EndpointTestGroup{
		renderTests,
		renderDiffTests,
		profileTypesTests,
		labelValuesTests,
		labelNamesTests,
		seriesTests,
		selectMergeStacktracesTests,
		selectMergeProfileTests,
		selectSeriesTests,
		selectMergeSpanProfileTests,
		diffTests,
	}

	EachPyroscopeTest(t, func(p *PyroscopeTest, t *testing.T) {
		client := http.DefaultClient
		isV1Test := strings.HasSuffix(t.Name(), "v1")

		for _, endpoint := range allTests {
			for wantCode, tests := range endpoint.Tests {
				for _, tt := range tests {
					t.Run(fmt.Sprintf("%s/%s", endpoint.Path, tt.Name), func(t *testing.T) {
						t.Parallel()

						path, err := url.JoinPath(p.URL(), endpoint.Path)
						require.NoError(t, err, "failed to create path")

						u, err := url.Parse(path)
						require.NoError(t, err, "failed to parse url")
						u.RawQuery = tt.Params.Encode()

						var reqBody io.Reader
						if tt.Body != "" {
							reqBody = bytes.NewReader([]byte(tt.Body))
						}

						req, err := http.NewRequest(tt.Method, u.String(), reqBody)
						require.NoError(t, err)

						for key, vals := range tt.Header {
							for _, val := range vals {
								req.Header.Add(key, val)
							}
						}

						res, err := client.Do(req)
						require.NoError(t, err)

						wantCode := wantCode
						if tt.WantV1StatusCode != 0 && isV1Test {
							wantCode = tt.WantV1StatusCode
						}

						if !assert.Equal(t, wantCode, res.StatusCode) {
							bytes, err := io.ReadAll(res.Body)
							res.Body.Close()
							if err != nil {
								t.Log("failed to read response body:", err)
							} else {
								t.Log("response body:", string(bytes))
							}
						}
					})
				}
			}
		}
	})
}
