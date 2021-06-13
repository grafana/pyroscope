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
	"reflect"
	"regexp"
	"strings"
	"unicode"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/util/slices"
)

// to run this program:
//   go run scripts/generate-sample-config/main.go -format yaml
//   go run scripts/generate-sample-config/main.go -format md
// or:
//   go run scripts/generate-sample-config/main.go -directory ../pyroscope.io/docs

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
		return
	}
	if err := ioutil.WriteFile(path, newContent, fs.FileMode(0)); err != nil {
		panic(err)
	}
}

func writeConfigDocs(w io.Writer, subcommand, format string) {
	flagSet := flag.NewFlagSet("pyroscope "+subcommand, flag.ExitOnError)
	var val interface{}
	switch subcommand {
	case "agent":
		val = new(config.Agent)
		cli.PopulateFlagSet(val, flagSet)
	case "server":
		val = new(config.Server)
		cli.PopulateFlagSet(val, flagSet)
	case "convert":
		val = new(config.Convert)
		cli.PopulateFlagSet(val, flagSet)
	case "exec":
		val = new(config.Exec)
		cli.PopulateFlagSet(val, flagSet, "pid")
	case "connect":
		val = new(config.Exec)
		cli.PopulateFlagSet(val, flagSet, "group-name", "user-name", "no-root-drop")
	case "target":
		val = new(config.Target)
		cli.PopulateFlagSet(val, flagSet)
	default:
		log.Fatalf("Unknown subcommand %q", subcommand)
	}

	sf := cli.NewSortedFlags(val, flagSet)
	switch format {
	case "yaml":
		_, _ = fmt.Fprintln(w, "---")
		sf.VisitAll(func(f *flag.Flag) {
			if f.Name == "config" {
				return
			}
			var v string
			if reflect.TypeOf(f.Value).Elem().Kind() == reflect.Slice {
				v = f.Value.String()
			} else {
				v = fmt.Sprintf("%q", f.Value)
			}
			_, _ = fmt.Fprintf(w, "# %s\n%s: %s\n\n", toPrettySentence(f.Usage), f.Name, v)
		})
	case "md":
		_, _ = fmt.Fprintf(w, "| %s | %s | %s |\n", "Name", "Default Value", "Usage")
		_, _ = fmt.Fprintf(w, "| %s | %s | %s |\n", ":-", ":-", ":-")
		sf.VisitAll(func(f *flag.Flag) {
			if f.Name == "config" {
				return
			}
			// Replace vertical bar glyph with HTML code.
			desc := strings.ReplaceAll(toPrettySentence(f.Usage), "|", `&#124;`)
			_, _ = fmt.Fprintf(w, "| %s | %s | %s |\n", f.Name, f.DefValue, desc)
		})
	default:
		logrus.Fatalf("Unknown format %q", format)
	}
}

// Capitalizes the first letter and adds period at the end, if necessary.
func toPrettySentence(s string) string {
	if s == "" {
		return ""
	}
	x := []rune(s)
	x[0] = unicode.ToUpper(x[0])
	if x[len(s)-1] != '.' {
		x = append(x, '.')
	}
	return string(x)
}
