package v2

import (
	_ "embed"
	"fmt"
	"html/template"
)

//go:embed tool.blocks.index.gohtml
var indexPageHtml string

//go:embed tool.blocks.list.gohtml
var blocksPageHtml string

//go:embed tool.blocks.detail.gohtml
var blockDetailsPageHtml string

//go:embed tool.blocks.dataset.gohtml
var datasetDetailsPageHtml string

//go:embed tool.blocks.dataset.profiles.gohtml
var datasetProfilesPageHtml string

//go:embed tool.blocks.profile.visualization.gohtml
var profileVisualizationPageHtml string

//go:embed tool.blocks.dataset.index.gohtml
var datasetIndexPageHtml string

type indexPageContent struct {
	Users []string
	Now   string
}

type blockListPageContent struct {
	User           string
	SelectedBlocks *blockListResult
	Query          *blockQuery
	Now            string
}

type blockDetailsPageContent struct {
	User        string
	Block       *blockDetails
	Shard       uint32
	BlockTenant string
	Now         string
}

type datasetDetailsPageContent struct {
	User        string
	BlockID     string
	Shard       uint32
	BlockTenant string
	Dataset     *datasetDetails
	Now         string
}

type datasetProfilesPageContent struct {
	User        string
	BlockID     string
	Shard       uint32
	BlockTenant string
	Dataset     *datasetDetails
	Profiles    []profileInfo
	Now         string
}

type templates struct {
	indexTemplate                *template.Template
	blocksTemplate               *template.Template
	blockDetailsTemplate         *template.Template
	datasetDetailsTemplate       *template.Template
	datasetProfilesTemplate      *template.Template
	profileVisualizationTemplate *template.Template
	datasetIndexTemplate         *template.Template
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
	datasetDetailsTemplate := template.New("dataset-details")
	template.Must(datasetDetailsTemplate.Parse(datasetDetailsPageHtml))
	datasetProfilesTemplate := template.New("dataset-profiles")
	template.Must(datasetProfilesTemplate.Parse(datasetProfilesPageHtml))
	profileVisualizationTemplate := template.New("profile-visualization").Funcs(template.FuncMap{
		"add":  add,
		"dict": dict,
	})
	template.Must(profileVisualizationTemplate.Parse(profileVisualizationPageHtml))
	datasetIndexTemplate := template.New("dataset-index").Funcs(template.FuncMap{
		"sub": sub,
	})
	template.Must(datasetIndexTemplate.Parse(datasetIndexPageHtml))
	t := &templates{
		indexTemplate:                indexTemplate,
		blocksTemplate:               blocksTemplate,
		blockDetailsTemplate:         blockDetailsTemplate,
		datasetDetailsTemplate:       datasetDetailsTemplate,
		datasetProfilesTemplate:      datasetProfilesTemplate,
		profileVisualizationTemplate: profileVisualizationTemplate,
		datasetIndexTemplate:         datasetIndexTemplate,
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

func sub(param1, param2 int) int {
	return param1 - param2
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

// dict creates a map for passing multiple values to a template
func dict(values ...interface{}) (map[string]interface{}, error) {
	if len(values)%2 != 0 {
		return nil, fmt.Errorf("dict requires an even number of arguments")
	}
	dict := make(map[string]interface{}, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict keys must be strings")
		}
		dict[key] = values[i+1]
	}
	return dict, nil
}
