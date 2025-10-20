package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/getkin/kin-openapi/openapi3"
)

func main() {
	var (
		inputDir     = flag.String("input", "api/connect-openapi/gen", "Directory containing OpenAPI YAML files")
		templateFile = flag.String("template", "docs/sources/reference-server-api/index.template", "Template flame used to generate markdown")
		outputFile   = flag.String("output", "docs/sources/reference-server-api/index.md", "Output file for generated markdown")
		help         = flag.Bool("help", false, "Show help")
	)
	flag.Parse()

	if *help {
		fmt.Println("API Documentation Generator")
		fmt.Println()
		fmt.Println("Generates unified API documentation from OpenAPI v3 YAML files.")
		fmt.Println("Processes all .yaml/.yml files in the input directory and creates")
		fmt.Println("a single markdown file using the provided template.")
		fmt.Println()
		fmt.Println("Only processes endpoints tagged with 'scope/public' and generates")
		fmt.Println("both cURL and Python code examples for each endpoint.")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Printf("  %s [flags]\n", os.Args[0])
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Printf("  %s\n", os.Args[0])
		fmt.Printf("  %s -input ./specs -output ./docs/api.md\n", os.Args[0])
		fmt.Println()
		fmt.Println("Flags:")
		flag.PrintDefaults()
		return
	}

	if err := generateDocs(*inputDir, *templateFile, *outputFile); err != nil {
		log.Fatalf("Error generating documentation: %v", err)
	}

	fmt.Printf("Documentation generated successfully: %s\n", *outputFile)
}

func generateDocs(inputDir, templateFile, outputFile string) error {
	// Find all OpenAPI YAML files
	yamlFiles, err := findYAMLFiles(inputDir)
	if err != nil {
		return fmt.Errorf("finding YAML files: %w", err)
	}

	if len(yamlFiles) == 0 {
		return fmt.Errorf("no YAML files found in %s", inputDir)
	}

	// Create output directory if needed
	outputDir := filepath.Dir(outputFile)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	// Parse all specs
	loader := openapi3.NewLoader()
	specs := make(map[string]*openapi3.T)
	for _, file := range yamlFiles {
		spec, err := loader.LoadFromFile(file)
		if err != nil {
			log.Printf("Warning: failed to parse %s: %v", file, err)
			continue
		}

		// Use relative path as key
		relPath, _ := filepath.Rel(inputDir, file)
		specs[relPath] = spec
	}

	// Generate single unified documentation file
	return generateUnifiedDoc(specs, templateFile, outputFile)
}

func findYAMLFiles(dir string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && (strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
			files = append(files, path)
		}
		return nil
	})

	return files, err
}

func generateUnifiedDoc(specs map[string]*openapi3.T, templateFile string, outputFile string) error {

	tmpl, err := template.ParseFiles(templateFile)
	if err != nil {
		return err
	}

	f, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer f.Close()

	err = tmpl.ExecuteTemplate(f, filepath.Base(templateFile), &templateSpecs{
		Specs: specs,
	})
	if err != nil {
		return err
	}

	return nil
}

type templateSpecs struct {
	Specs map[string]*openapi3.T
}

func (s *templateSpecs) RenderAPIGroup(path string) string {
	md := &strings.Builder{}

	paths := make(map[string]*openapi3.PathItem)
	pathKeys := make([]string, 0)
	for _, s := range s.Specs {
		for p, pItem := range s.Paths.Map() {
			if strings.HasPrefix(p, path) {
				_, ok := paths[p]
				if ok {
					panic(fmt.Sprintf("path %s already exists", p))
				}
				op := pItem.Get
				if op == nil {
					op = pItem.Post
				}
				if op == nil {
					continue
				}

				// skip non public ones
				public := false
				for _, t := range op.Tags {
					if t == "scope/public" {
						public = true
					}
				}
				if !public {
					continue
				}

				// now add them
				paths[p] = pItem
				pathKeys = append(pathKeys, p)
			}
		}
		if len(pathKeys) > 0 {
			break
		}
	}
	if len(paths) == 0 {
		panic(fmt.Sprintf(`no paths found for "%s"`, path))
	}
	sort.Strings(pathKeys)

	for _, p := range pathKeys {
		pItem := paths[p]
		fmt.Fprintf(md, "#### `%s`\n\n", p)
		fmt.Fprintf(md, "%s\n\n", pItem.Post.Description)

		s.writeParameters(md, pItem.Post)
		s.writeExamples(md, p, pItem.Post)

		// TODO Responses
	}

	return md.String()
}

func (s *templateSpecs) writeParameters(sb io.Writer, op *openapi3.Operation) {
	if op.RequestBody == nil {
		panic("no request body")
	}

	requestSchema := requestBodySchemaFrom(op)

	fmt.Fprintln(sb, "A request body with the following fields is required:")
	fmt.Fprintln(sb, "")
	fmt.Fprintln(sb, "|Field | Description | Example |")
	fmt.Fprintln(sb, "|:-----|:------------|:--------|")

	writeSchema(sb, requestSchema)
	fmt.Fprintln(sb, "")
}

func cleanTableField(s string) (string, bool) {
	if strings.Contains(s, "[hidden]") {
		return "", false
	}
	return strings.ReplaceAll(s, "\n", " "), true
}

func getExample(schema *openapi3.Schema) (string, any) {
	if schema.Extensions != nil {
		examples := schema.Extensions["examples"].([]any)
		if len(examples) > 0 {
			switch examples[0].(type) {
			case string:
				return examples[0].(string), examples[0]
			case []any:
				res, err := json.Marshal(examples[0])
				if err != nil {
					panic(err)
				}
				return string(res), examples[0]
			default:
				panic(fmt.Sprintf("unknown example type: %T", examples[0]))
			}
		}
	}
	return "", nil
}

func collectParameters(schema *openapi3.Schema, prefix string, fn func(prefix string, name string, schema *openapi3.Schema)) {
	fnames := make([]string, 0, len(schema.Properties))

	for f := range schema.Properties {
		fnames = append(fnames, f)
	}

	sort.Slice(fnames, func(i, j int) bool {
		// put start/end at the beginning
		for _, f := range []string{"start", "end"} {
			if fnames[i] == f {
				return true
			}
			if fnames[j] == f {
				return false
			}
		}

		return fnames[i] < fnames[j]
	})

	for _, f := range fnames {
		fSchema := schema.Properties[f].Value
		if fSchema.Type.Is(openapi3.TypeObject) {
			collectParameters(fSchema, prefix+f+".", fn)
			continue
		}
		if fSchema.Type.Is(openapi3.TypeArray) && fSchema.Items.Value.Type.Is(openapi3.TypeObject) {
			collectParameters(fSchema.Items.Value, prefix+f+"[].", fn)
			continue
		}
		fn(prefix, f, fSchema)
	}
}

func writeSchema(sb io.Writer, schema *openapi3.Schema) {
	collectParameters(schema, "", func(prefix string, name string, schema *openapi3.Schema) {
		description, keep := cleanTableField(schema.Description)
		if !keep {
			return
		}
		example, _ := getExample(schema)
		if example != "" {
			example = fmt.Sprintf("`%s`", example)
		}
		fmt.Fprintf(sb, "|`%s%s` | %s | %s |\n", prefix, name, description, example)
	})
}

func requestBodySchemaFrom(op *openapi3.Operation) *openapi3.Schema {
	return op.RequestBody.Value.Content["application/json"].Schema.Value
}

type exampleValues struct {
	Curl   any
	Python any
}

var exampleParameters = map[string]exampleValues{
	// start
	"1676282400000": {
		Curl:   shellCmd("$(expr $(date +%s) - 3600 )000"),
		Python: pythonExpr("int((datetime.datetime.now()- datetime.timedelta(hours = 1)).timestamp() * 1000)"),
	},
	// end
	"1676289600000": {
		Curl:   shellCmd("$(date +%s)000"),
		Python: pythonExpr("int(datetime.datetime.now().timestamp() * 1000)"),
	},
	"PROFILE_BASE64": {
		Curl:   shellCmd(`"$(cat cpu.pb.gz| base64 -w 0)"`),
		Python: pythonExpr(`base64.b64encode(open('cpu.pb.gz', 'rb').read()).decode('ascii')`),
	},
}

type exampleParams struct {
	url string
}

type exampler interface {
	render(io.Writer, *exampleParams)
	name() string
}

func (s *templateSpecs) writeExamples(sb io.Writer, path string, op *openapi3.Operation) {
	params := &exampleParams{
		url: "http://localhost:4040" + path,
	}

	fmt.Fprintln(sb, "{{% code %}}")

	for _, ex := range []exampler{
		newExampleCurl(requestBodySchemaFrom(op)),
		newExamplePython(requestBodySchemaFrom(op)),
	} {
		fmt.Fprintf(sb, "```%s\n", ex.name())
		ex.render(sb, params)
		fmt.Fprintln(sb, "```")
		fmt.Fprintln(sb, "")
	}

	fmt.Fprintln(sb, "{{% /code %}}")
}

func setBody(body map[string]any, prefix string, name string, value any) {
	prefixParts := strings.Split(prefix, ".")
	result := body
	for _, part := range prefixParts {
		if part == "" {
			continue
		}

		// handle array
		if strings.HasSuffix(part, "[]") {
			part = part[:len(part)-2]

			var v []map[string]any
			vInt, ok := result[part]
			if !ok {
				v = []map[string]any{{}}
				result[part] = v
			} else {
				v = vInt.([]map[string]any)
			}

			if len(v) != 1 {
				panic("unexpected length of array")
			}

			result = v[0]
			continue
		}
		value, ok := result[part]
		if !ok {
			value = map[string]any{}
			result[part] = value
		}
		result = value.(map[string]any)
	}
	result[name] = value
}

func addLabelsToSeries(body map[string]any, lbls ...string) {
	if len(lbls)%2 != 0 {
		panic("labels must be pairs")
	}

	series, ok := body["series"]
	if !ok {
		return
	}

	seriesList, ok := series.([]map[string]any)
	if !ok {
		return
	}

	for _, s := range seriesList {
		labels, ok := s["labels"]
		if !ok {
			continue
		}

		labelsList, ok := labels.([]map[string]any)
		if !ok {
			continue
		}

		lbs := make([]map[string]any, 0, len(lbls)/2)
		for i := 0; i < len(lbls); i += 2 {
			lbs = append(lbs, map[string]any{"name": lbls[i], "value": lbls[i+1]})
		}

		s["labels"] = append(lbs, labelsList...)
	}
}
