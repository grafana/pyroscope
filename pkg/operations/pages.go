package operations

import (
	_ "embed"
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
	SelectedBlocks []*blockGroup
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
	blocksTemplate := template.New("blocks")
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
