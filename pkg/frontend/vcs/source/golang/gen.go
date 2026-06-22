//go:build ignore

package main

import (
	"bytes"
	"go/format"
	"html/template"
	"log"
	"os"

	"github.com/grafana/pyroscope/v2/pkg/frontend/vcs/source/golang"
)

func main() {
	// todo: In the future we might want to support more than one version
	// Or even list all files from standard packages to improve matching.
	packages, err := golang.StdPackages("")
	if err != nil {
		log.Fatal(err)
	}
	t := template.Must(template.New("packages").Parse(packagesTemplate))
	var buff bytes.Buffer
	if err = t.Execute(&buff, packages); err != nil {
		log.Fatal(err)
	}
	formatted, err := format.Source(buff.Bytes())
	if err != nil {
		log.Fatal(err)
	}
	err = os.WriteFile("packages_gen.go", formatted, 0666)
	if err != nil {
		log.Fatal(err)
	}
}

var packagesTemplate = `
package golang
// Code generated. DO NOT EDIT.

var StandardPackages = map[string]struct{}{
	{{- range $key, $value := .}}
	"{{$key}}": {},
	{{- end}}
}
`
