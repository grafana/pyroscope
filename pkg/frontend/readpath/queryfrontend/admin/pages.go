package admin

import (
	_ "embed"
	"html/template"
	"time"
)

//go:embed query_diagnostics.gohtml
var diagnosticsPageHtml string

//go:embed diagnostics_list.gohtml
var diagnosticsListPageHtml string

// diagnosticsListPageContent contains data for the diagnostics list page shell.
type diagnosticsListPageContent struct {
	Now time.Time
}

// diagnosticsPageContent contains data for the diagnostics page shell.
type diagnosticsPageContent struct {
	Now time.Time
}

type templates struct {
	diagnosticsTemplate     *template.Template
	diagnosticsListTemplate *template.Template
}

var pageTemplates = initTemplates()

func initTemplates() *templates {
	diagnosticsTemplate := template.New("diagnostics")
	template.Must(diagnosticsTemplate.Parse(diagnosticsPageHtml))

	diagnosticsListTemplate := template.New("diagnostics-list")
	template.Must(diagnosticsListTemplate.Parse(diagnosticsListPageHtml))

	return &templates{
		diagnosticsTemplate:     diagnosticsTemplate,
		diagnosticsListTemplate: diagnosticsListTemplate,
	}
}
