package querydiagnostics

import (
	_ "embed"
	"html/template"
	"time"
)

//go:embed query_diagnostics.gohtml
var diagnosticsPageHtml string

//go:embed diagnostics_list.gohtml
var diagnosticsListPageHtml string

//go:embed import_blocks.gohtml
var importBlocksPageHtml string

type pageContent struct {
	Now time.Time
}

type templates struct {
	diagnosticsTemplate     *template.Template
	diagnosticsListTemplate *template.Template
	importBlocksTemplate    *template.Template
}

var pageTemplates = initTemplates()

func initTemplates() *templates {
	diagnosticsTemplate := template.New("diagnostics")
	template.Must(diagnosticsTemplate.Parse(diagnosticsPageHtml))

	diagnosticsListTemplate := template.New("diagnostics-list")
	template.Must(diagnosticsListTemplate.Parse(diagnosticsListPageHtml))

	importBlocksTemplate := template.New("import-blocks")
	template.Must(importBlocksTemplate.Parse(importBlocksPageHtml))

	return &templates{
		diagnosticsTemplate:     diagnosticsTemplate,
		diagnosticsListTemplate: diagnosticsListTemplate,
		importBlocksTemplate:    importBlocksTemplate,
	}
}
