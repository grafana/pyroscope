package admin

import (
	_ "embed"
	"html/template"
	"strings"
	"time"
)

//go:embed query_diagnostics.gohtml
var diagnosticsPageHtml string

type diagnosticsPageContent struct {
	Now time.Time

	Tenants       []string
	TenantID      string
	StartTime     string
	EndTime       string
	QueryType     string
	LabelSelector string

	PlanJSON          string
	PlanTree          *PlanTreeNode
	MetadataStats     string
	MetadataQueryTime time.Duration

	QueryResponseTime time.Duration
	ReportStats       string

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

type templates struct {
	diagnosticsTemplate *template.Template
}

var templateFuncs = template.FuncMap{
	"lower": strings.ToLower,
	"subtract": func(a, b int) int {
		return a - b
	},
}

var pageTemplates = initTemplates()

func initTemplates() *templates {
	diagnosticsTemplate := template.New("diagnostics").Funcs(templateFuncs)
	template.Must(diagnosticsTemplate.Parse(diagnosticsPageHtml))
	return &templates{
		diagnosticsTemplate: diagnosticsTemplate,
	}
}
