package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/pprof/profile"
	"github.com/invopop/jsonschema"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	"github.com/grafana/pyroscope/pkg/frontend/dot/graph"
	"github.com/grafana/pyroscope/pkg/frontend/dot/report"
)

const fetchPyroscopeProfileToolPrompt = `
Fetches a profile from Pyroscope for a given time range. By default, the time range is the past 1 hour.
The profile type is required (e.g., "process_cpu:cpu:nanoseconds:cpu:nanoseconds"). 
Matchers are optional but recommended - they are generally used to select an application by the service_name
label (e.g. {service_name="foo"}). The returned profile is in DOT format which can be used to understand
the application's performance characteristics and identify hot spots.
`

// FetchPyroscopeProfileParams are the parameters for the fetch_pyroscope_profile tool.
type FetchPyroscopeProfileParams struct {
	ProfileType  string `json:"profile_type" jsonschema:"required,description=The profile type (e.g. process_cpu:cpu:nanoseconds:cpu:nanoseconds)"`
	Matchers     string `json:"matchers,omitempty" jsonschema:"description=Prometheus style matchers used to filter the result set (defaults to: {})"`
	MaxNodeDepth int    `json:"max_node_depth,omitempty" jsonschema:"description=Maximum depth of nodes in the resulting profile. Less depth results in smaller profiles. Default: 100"`
	StartRFC3339 string `json:"start_rfc_3339,omitempty" jsonschema:"description=Start time in RFC3339 format (defaults to 1 hour ago)"`
	EndRFC3339   string `json:"end_rfc_3339,omitempty" jsonschema:"description=End time in RFC3339 format (defaults to now)"`
}

// Tools holds the MCP tools and their dependencies.
type Tools struct {
	querierClient querierv1connect.QuerierServiceClient
}

// NewTools creates a new Tools instance with the given querier client.
func NewTools(querierClient querierv1connect.QuerierServiceClient) *Tools {
	return &Tools{
		querierClient: querierClient,
	}
}

// RegisterTools registers all Pyroscope MCP tools with the server.
func (t *Tools) RegisterTools(s *server.MCPServer) {
	tool, handler := t.createFetchProfileTool()
	s.AddTool(tool, handler)
}

func (t *Tools) createFetchProfileTool() (mcp.Tool, server.ToolHandlerFunc) {
	schema := createJSONSchema[FetchPyroscopeProfileParams]()
	properties := make(map[string]any, schema.Properties.Len())
	for pair := schema.Properties.Oldest(); pair != nil; pair = pair.Next() {
		properties[pair.Key] = pair.Value
	}

	schemaBytes, _ := json.Marshal(mcp.ToolArgumentsSchema{
		Type:       schema.Type,
		Properties: properties,
		Required:   schema.Required,
	})

	tool := mcp.Tool{
		Name:           "fetch_pyroscope_profile",
		Description:    fetchPyroscopeProfileToolPrompt,
		RawInputSchema: schemaBytes,
	}

	handler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var params FetchPyroscopeProfileParams
		argBytes, err := json.Marshal(request.Params.Arguments)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal arguments: %w", err)
		}
		if err := json.Unmarshal(argBytes, &params); err != nil {
			return nil, fmt.Errorf("failed to unmarshal arguments: %w", err)
		}

		result, err := t.fetchPyroscopeProfile(ctx, params)
		if err != nil {
			return nil, err
		}

		return mcp.NewToolResultText(result), nil
	}

	return tool, handler
}

func (t *Tools) fetchPyroscopeProfile(ctx context.Context, params FetchPyroscopeProfileParams) (string, error) {
	// Set defaults
	matchers := params.Matchers
	if strings.TrimSpace(matchers) == "" {
		matchers = "{}"
	}
	// Ensure matchers are wrapped in braces
	matchersRegex := regexp.MustCompile(`^\{.*\}$`)
	if !matchersRegex.MatchString(matchers) {
		matchers = fmt.Sprintf("{%s}", matchers)
	}

	maxNodeDepth := params.MaxNodeDepth
	if maxNodeDepth == 0 {
		maxNodeDepth = 100
	}

	// Parse time range
	start, err := parseRFC3339OrDefault(params.StartRFC3339, time.Time{})
	if err != nil {
		return "", fmt.Errorf("failed to parse start timestamp %q: %w", params.StartRFC3339, err)
	}

	end, err := parseRFC3339OrDefault(params.EndRFC3339, time.Time{})
	if err != nil {
		return "", fmt.Errorf("failed to parse end timestamp %q: %w", params.EndRFC3339, err)
	}

	start, end, err = validateTimeRange(start, end)
	if err != nil {
		return "", err
	}

	// Build the query - profile type with matchers
	labelSelector := matchers

	// We use a higher source profile max nodes to get more detail
	sourceProfileMaxNodes := int64(512)
	dotProfileMaxNodes := int64(maxNodeDepth)
	if dotProfileMaxNodes > sourceProfileMaxNodes {
		sourceProfileMaxNodes = dotProfileMaxNodes
	}

	// Call the querier
	resp, err := t.querierClient.SelectMergeProfile(ctx, connect.NewRequest(&querierv1.SelectMergeProfileRequest{
		Start:         start.UnixMilli(),
		End:           end.UnixMilli(),
		ProfileTypeID: params.ProfileType,
		LabelSelector: labelSelector,
		MaxNodes:      &sourceProfileMaxNodes,
	}))
	if err != nil {
		return "", fmt.Errorf("failed to query Pyroscope: %w", err)
	}

	// Convert to DOT format
	var buf bytes.Buffer
	if err := pprofToDotProfile(&buf, resp.Msg, int(dotProfileMaxNodes)); err != nil {
		return "", fmt.Errorf("failed to convert profile to DOT: %w", err)
	}

	result := cleanupDotProfile(buf.String())
	if strings.Contains(result, "Showing nodes accounting for 0, 0% of 0 total") {
		return "", fmt.Errorf("pyroscope returned an empty profile")
	}

	return result, nil
}

func pprofToDotProfile(w *bytes.Buffer, p interface{ MarshalVT() ([]byte, error) }, maxNodes int) error {
	data, err := p.MarshalVT()
	if err != nil {
		return fmt.Errorf("failed to marshal profile: %w", err)
	}
	pr, err := profile.ParseData(data)
	if err != nil {
		return fmt.Errorf("failed to parse profile: %w", err)
	}
	rpt := report.NewDefault(pr, report.Options{NodeCount: maxNodes})
	gr, cfg := report.GetDOT(rpt)
	graph.ComposeDot(w, gr, &graph.DotAttributes{}, cfg)
	return nil
}

func parseRFC3339OrDefault(s string, def time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return def, nil
	}
	return time.Parse(time.RFC3339, s)
}

func validateTimeRange(start, end time.Time) (time.Time, time.Time, error) {
	if end.IsZero() {
		end = time.Now()
	}
	if start.IsZero() {
		start = end.Add(-1 * time.Hour)
	}
	if start.After(end) || start.Equal(end) {
		return time.Time{}, time.Time{}, fmt.Errorf("start timestamp %q must be strictly before end timestamp %q",
			start.Format(time.RFC3339), end.Format(time.RFC3339))
	}
	return start, end, nil
}

// cleanupDotProfile removes verbose attributes from the DOT profile to reduce size.
var cleanupRegex = regexp.MustCompile(`(?m)(fontsize=\d+ )|(id="node\d+" )|(labeltooltip=".*?\)" )|(tooltip=".*?\)" )|(N\d+ -> N\d+).*|(N\d+ \[label="other.*\n)|(shape=box )|(fillcolor="#\w{6}")|(color="#\w{6}" )`)

func cleanupDotProfile(profile string) string {
	return cleanupRegex.ReplaceAllStringFunc(profile, func(match string) string {
		// Preserve edge labels (e.g., "N1 -> N2")
		if m := regexp.MustCompile(`^N\d+ -> N\d+`).FindString(match); m != "" {
			return m
		}
		return ""
	})
}

// createJSONSchema creates a JSON schema from a struct type using reflection.
func createJSONSchema[T any]() *jsonschema.Schema {
	var zero T
	reflector := jsonschema.Reflector{
		Anonymous:                  true,
		AllowAdditionalProperties:  true,
		RequiredFromJSONSchemaTags: true,
		DoNotReference:             true,
		ExpandedStruct:             true,
	}
	return reflector.ReflectFromType(reflect.TypeOf(zero))
}

