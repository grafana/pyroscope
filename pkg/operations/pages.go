package operations

import (
	_ "embed"
	"fmt"
	"html/template"

	"github.com/grafana/pyroscope/pkg/phlaredb/bucketindex"
)

//go:embed tool.blocks.index.gohtml
var indexPageHtml string

//go:embed tool.blocks.list.gohtml
var blocksPageHtml string

//go:embed tool.blocks.detail.gohtml
var blockDetailsPageHtml string

type indexPageContent struct {
	Users []string
	Now   string
}

type blockListPageContent struct {
	User           string
	Index          *bucketindex.Index
	SelectedPeriod string
	SelectedBlocks *blockListResult
	Query          *blockQuery
	Now            string
}

type blockDetailsPageContent struct {
	User  string
	Block *blockDetails
	Now   string
}

type templates struct {
	indexTemplate        *template.Template
	blocksTemplate       *template.Template
	blockDetailsTemplate *template.Template
}

var pageTemplates = initTemplates()

func initTemplates() *templates {
	indexTemplate := template.New("index")
	template.Must(indexTemplate.Parse(indexPageHtml))
	blocksTemplate := template.New("blocks").Funcs(template.FuncMap{
		"mul":    mul,
		"add":    add,
		"addf":   addf,
		"subf":   subf,
		"mulf":   mulf,
		"divf":   divf,
		"format": format,
		"float":  float,
	})
	template.Must(blocksTemplate.Parse(blocksPageHtml))
	blockDetailsTemplate := template.New("block-details")
	template.Must(blockDetailsTemplate.Parse(blockDetailsPageHtml))
	t := &templates{
		indexTemplate:        indexTemplate,
		blocksTemplate:       blocksTemplate,
		blockDetailsTemplate: blockDetailsTemplate,
	}
	return t
}

func mul(param1, param2 int) int {
	return param1 * param2
}

func mulf(param1, param2 float64) float64 {
	return param1 * param2
}

func add(param1, param2 int) int {
	return param1 + param2
}

func addf(param1, param2 float64) float64 {
	return param1 + param2
}

func subf(param1, param2 float64) float64 {
	return param1 - param2
}

func divf(param1, param2 int) float64 {
	return float64(param1) / float64(param2)
}

func format(format string, value any) string {
	return fmt.Sprintf(format, value)
}

func float(param int) float64 {
	return float64(param)
}
