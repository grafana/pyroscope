package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/util/slices"
	"github.com/sirupsen/logrus"
)

// to run this program:
//   go run scripts/generate-sample-config/main.go -format yaml
//   go run scripts/generate-sample-config/main.go -format md
// or:
//   go run scripts/generate-sample-config/main.go -directory ../pyroscope.io/docs

var cfg config.Config

func main() {
	var (
		format     string
		subcommand string
		directory  string
	)
	flag.StringVar(&format, "format", "yaml", "yaml or md")
	flag.StringVar(&subcommand, "subcommand", "server", "server, agent, exec...")
	flag.StringVar(&directory, "directory", "", "directory to scan and perform auto replacements")
	flag.Parse()

	if directory != "" {
		err := filepath.Walk(directory, func(path string, f os.FileInfo, err error) error {
			if slices.StringContains([]string{".mdx", ".md"}, filepath.Ext(path)) {
				// log.Println(""path)
				processFile(path)
			}
			return nil
		})
		if err != nil {
			panic(err)
		}
	} else {
		writeConfigDocs(os.Stdout, subcommand, format)
	}
}

func processFile(path string) {
	log.Printf("processing %s", path)
	content, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}

	// r := regexp.MustCompile("<!-- generate-sample-config:.+?:.+? -->.+?<!-- \\/generate-sample-config -->")
	r := regexp.MustCompile("(?s)<!--\\s*generate-sample-config:.+?:.+?\\s*-->.*?<!--\\s*/generate-sample-config\\s*-->")
	r2 := regexp.MustCompile("generate-sample-config:(.+?):(.+?)\\s*-")
	newContent := r.ReplaceAllFunc(content, func(b []byte) []byte {
		submatches := r2.FindSubmatch(b)
		buf := bytes.Buffer{}

		subcommand := string(submatches[1])
		format := string(submatches[2])

		fmt.Fprintf(&buf, "<!-- generate-sample-config:%s:%s -->\n", subcommand, format)
		if format == "yaml" {
			fmt.Fprintf(&buf, "```yaml\n")
		}
		writeConfigDocs(&buf, subcommand, format)
		if format == "yaml" {
			fmt.Fprintf(&buf, "```\n")
		}
		fmt.Fprintf(&buf, "<!-- /generate-sample-config -->")
		return buf.Bytes()
	})

	if bytes.Equal(content, newContent) {
		log.Println("no changes")
	} else {
		ioutil.WriteFile(path, newContent, fs.FileMode(0))
	}

}
func writeConfigDocs(w io.Writer, subcommand, format string) {
	var val interface{}
	if subcommand == "agent" {
		val = &cfg.Agent
	} else if subcommand == "server" {
		val = &cfg.Server
	} else if subcommand == "convert" {
		val = &cfg.Convert
	} else if subcommand == "exec" {
		val = &cfg.Exec
	}

	flagSet := flag.NewFlagSet("pyroscope "+subcommand, flag.ExitOnError)

	cli.PopulateFlagSet(val, flagSet)
	sf := cli.NewSortedFlags(val, flagSet)
	if format == "yaml" {
		fmt.Fprintln(w, "---")
		sf.VisitAll(func(f *flag.Flag) {
			if f.Name != "config" {
				fmt.Fprintf(w, "# %s\n%s: %q\n\n", f.Usage, f.Name, f.DefValue)
			}
		})
	} else if format == "md" {
		fmt.Fprintf(w, "| %s | %s | %s |\n", "Name", "Default Value", "Usage")
		fmt.Fprintf(w, "| %s | %s | %s |\n", ":-", ":-", ":-")
		sf.VisitAll(func(f *flag.Flag) {
			if f.Name != "config" {
				fmt.Fprintf(w, "| %s | %s | %q |\n", f.Name, f.DefValue, f.Usage)
			}
		})
	} else {
		logrus.Fatalf("Unknown format %q", format)
	}
}
