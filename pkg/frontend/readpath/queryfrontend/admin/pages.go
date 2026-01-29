package admin

import (
	_ "embed"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/grafana/pyroscope/pkg/frontend/readpath/queryfrontend/diagnostics"
)

//go:embed query_diagnostics.gohtml
var diagnosticsPageHtml string

//go:embed diagnostics_list.gohtml
var diagnosticsListPageHtml string

// diagnosticsListPageContent contains data for the diagnostics list page.
type diagnosticsListPageContent struct {
	Now            time.Time
	Tenants        []string
	SelectedTenant string
	Diagnostics    []*diagnostics.DiagnosticSummary
	Error          string
}

type diagnosticsPageContent struct {
	Now time.Time

	Tenants       []string
	TenantID      string
	StartTime     string
	EndTime       string
	QueryType     string
	LabelSelector string

	// Query-type specific parameters
	// PPROF/TREE: MaxNodes
	MaxNodes string
	// LABEL_VALUES: LabelName
	LabelName string
	// SERIES_LABELS: LabelNames (comma-separated)
	SeriesLabelNames string
	// TIME_SERIES: Step, GroupBy, Limit
	Step    string
	GroupBy string
	Limit   string

	PlanJSON          string
	PlanTree          *PlanTreeNode
	MetadataStats     string
	MetadataQueryTime time.Duration

	QueryResponseTime time.Duration
	ExecutionTree     *ExecutionTreeNode
	DiagnosticsID     string // ID for retrieving stored diagnostics

	Error string
}

// PlanTreeNode represents a node in the visual query plan tree.
type PlanTreeNode struct {
	Type        string          // "MERGE" or "READ"
	Children    []*PlanTreeNode // Child nodes (for MERGE)
	BlockCount  int             // Number of blocks (for READ)
	Blocks      []PlanTreeBlock // Block info (for READ, limited for display)
	TotalBlocks int             // Total blocks in subtree
}

// PlanTreeBlock represents a block in the visual query plan tree.
type PlanTreeBlock struct {
	ID              string
	Shard           uint32
	Size            string // Human-readable size
	CompactionLevel uint32
}

// ExecutionTreeNode represents a node in the execution trace visualization.
type ExecutionTreeNode struct {
	Type             string
	Executor         string
	Duration         time.Duration
	DurationStr      string
	RelativeStart    time.Duration // Time offset from query start
	RelativeStartStr string
	Children         []*ExecutionTreeNode
	Stats            *ExecutionTreeStats
	Error            string
}

// ExecutionTreeStats contains stats for READ node execution.
type ExecutionTreeStats struct {
	BlocksRead        int64
	DatasetsProcessed int64
	BlockExecutions   []*BlockExecutionInfo
}

// BlockExecutionInfo contains execution details for a single block.
type BlockExecutionInfo struct {
	BlockID           string
	Duration          time.Duration
	DurationStr       string
	RelativeStart     time.Duration
	RelativeStartStr  string
	RelativeEnd       time.Duration
	RelativeEndStr    string
	DatasetsProcessed int64
	Size              string // Human-readable size
	Shard             uint32
	CompactionLevel   uint32
}

type templates struct {
	diagnosticsTemplate     *template.Template
	diagnosticsListTemplate *template.Template
}

var templateFuncs = template.FuncMap{
	"lower": strings.ToLower,
	"subtract": func(a, b int) int {
		return a - b
	},
	"formatTime": func(t time.Time) string {
		return t.Format("2006-01-02 15:04:05")
	},
	"formatMs": func(ms int64) string {
		if ms < 1000 {
			return fmt.Sprintf("%dms", ms)
		}
		return fmt.Sprintf("%.2fs", float64(ms)/1000)
	},
	"formatDuration": func(d time.Duration) string {
		if d < time.Millisecond {
			return fmt.Sprintf("%.3fms", float64(d.Nanoseconds())/1e6)
		}
		if d < time.Second {
			return fmt.Sprintf("%.3fms", float64(d.Nanoseconds())/1e6)
		}
		return fmt.Sprintf("%.3fs", d.Seconds())
	},
	"formatTimeRange": func(startMs, endMs int64) string {
		start := time.UnixMilli(startMs).UTC()
		end := time.UnixMilli(endMs).UTC()
		duration := end.Sub(start)
		if duration < time.Hour {
			return fmt.Sprintf("%s (%s)", start.Format("15:04:05"), formatDurationShort(duration))
		}
		return fmt.Sprintf("%s to %s", start.Format("Jan 2 15:04"), end.Format("Jan 2 15:04"))
	},
}

func formatDurationShort(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%.3fms", float64(d.Nanoseconds())/1e6)
	}
	if d < time.Second {
		return fmt.Sprintf("%.3fms", float64(d.Nanoseconds())/1e6)
	}
	return fmt.Sprintf("%.3fs", d.Seconds())
}

var pageTemplates = initTemplates()

func initTemplates() *templates {
	diagnosticsTemplate := template.New("diagnostics").Funcs(templateFuncs)
	template.Must(diagnosticsTemplate.Parse(diagnosticsPageHtml))

	diagnosticsListTemplate := template.New("diagnostics-list").Funcs(templateFuncs)
	template.Must(diagnosticsListTemplate.Parse(diagnosticsListPageHtml))

	return &templates{
		diagnosticsTemplate:     diagnosticsTemplate,
		diagnosticsListTemplate: diagnosticsListTemplate,
	}
}
