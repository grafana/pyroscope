//go:build ignore

package main

import (
	"bytes"
	"context"
	"html/template"
	"io"
	"log"
	"os"

	"github.com/github/go-pipe/pipe"

	"github.com/grafana/pyroscope/pkg/querier/golang"
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
	p := pipe.New(pipe.WithStdout(&buff))
	p.Add(
		pipe.Function("", func(ctx context.Context, env pipe.Env, stdin io.Reader, stdout io.Writer) error {
			err = t.Execute(stdout, packages)
			if err != nil {
				log.Fatal(err)
			}
			return nil
		}),
		// This might be a bit overkill, but it's nice to have a consistent format.
		// todo: We could use "go/format" package only that will simplify the code but also remove the needs
		// for expected installed binaries.
		pipe.Command("gofmt"),
		pipe.Command("goimports"),
	)
	p.Run(context.Background())
	err = os.WriteFile("packages_gen.go", buff.Bytes(), 0666)
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
