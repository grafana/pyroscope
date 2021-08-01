package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"unicode"

	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"

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
				return processFile(path)
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

var (
	sectionRegexp = regexp.MustCompile(`(?s)<!--\s*generate-sample-config:(.+?):(.+?)\s*-->.*?<!--\s*/generate-sample-config\s*-->`)
	headerRegexp  = regexp.MustCompile("generate-sample-config:(.+?):(.+?)\\s*-")
)

func processFile(path string) error {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	newContent := sectionRegexp.ReplaceAllFunc(content, func(b []byte) []byte {
		submatches := headerRegexp.FindSubmatch(b)
		buf := bytes.Buffer{}
		subcommand := string(submatches[1])
		format := string(submatches[2])
		writeConfigDocs(&buf, subcommand, format)
		return buf.Bytes()
	})

	if bytes.Equal(content, newContent) {
		return nil
	}

	fmt.Println(path)
	return ioutil.WriteFile(path, newContent, 0640)
}

func writeConfigDocs(w io.Writer, subcommand, format string) {
	flagSet := pflag.NewFlagSet("pyroscope "+subcommand, pflag.ExitOnError)
	opts := []cli.FlagOption{
		cli.WithReplacement("<supportedProfilers>", "pyspy, rbspy, phpspy, dotnetspy, ebpfspy"),
		cli.WithSkipDeprecated(true),
	}

	var val interface{}
	switch subcommand {
	case "agent":
		val = new(config.Agent)
		// Skip `targets` only from CLI reference.
		if format == "md" {
			cli.PopulateFlagSet(val, flagSet, append(opts, cli.WithSkip("targets"))...)
		} else {
			cli.PopulateFlagSet(val, flagSet, opts...)
		}
	case "server":
		val = new(config.Server)
		// Skip `metric-export-rules` only from CLI reference.
		if format == "md" {
			cli.PopulateFlagSet(val, flagSet, append(opts, cli.WithSkip("metric-export-rules"))...)
		} else {
			cli.PopulateFlagSet(val, flagSet, opts...)
		}
	case "convert":
		val = new(config.Convert)
		cli.PopulateFlagSet(val, flagSet, opts...)
	case "exec":
		val = new(config.Exec)
		cli.PopulateFlagSet(val, flagSet, append(opts, cli.WithSkip("pid"))...)
	case "connect":
		val = new(config.Exec)
		cli.PopulateFlagSet(val, flagSet, append(opts, cli.WithSkip("group-name", "user-name", "no-root-drop"))...)
	case "target":
		val = new(config.Target)
		cli.PopulateFlagSet(val, flagSet, append(opts, cli.WithSkip("tags"))...)
	case "metric-export-rule":
		val = new(config.MetricExportRule)
		cli.PopulateFlagSet(val, flagSet, opts...)
	default:
		log.Fatalf("Unknown subcommand %q", subcommand)
	}

	_, _ = fmt.Fprintf(w, "<!-- generate-sample-config:%s:%s -->\n", subcommand, format)

	switch format {
	case "yaml":
		writeYaml(w, flagSet)
	case "md":
		writeMarkdown(w, flagSet)
	default:
		logrus.Fatalf("Unknown format %q", format)
	}

	_, _ = fmt.Fprintf(w, "<!-- /generate-sample-config -->")
}

func writeYaml(w io.Writer, flagSet *pflag.FlagSet) {
	_, _ = fmt.Fprintf(w, "```yaml\n---\n")
	flagSet.VisitAll(func(f *pflag.Flag) {
		if f.Name == "config" {
			return
		}
		var v string
		switch reflect.TypeOf(f.Value).Elem().Kind() {
		case reflect.Slice, reflect.Map:
			v = f.Value.String()
		default:
			v = fmt.Sprintf("%q", f.Value)
		}
		_, _ = fmt.Fprintf(w, "# %s\n%s: %s\n\n", toPrettySentence(f.Usage), f.Name, v)
	})
	_, _ = fmt.Fprintf(w, "```\n")
}

func writeMarkdown(w io.Writer, flagSet *pflag.FlagSet) {
	_, _ = fmt.Fprintf(w, "| %s | %s | %s |\n", "Name", "Default Value", "Usage")
	_, _ = fmt.Fprintf(w, "| %s | %s | %s |\n", ":-", ":-", ":-")
	flagSet.VisitAll(func(f *pflag.Flag) {
		if f.Name == "config" {
			return
		}
		// Replace vertical bar glyph with HTML code.
		desc := strings.ReplaceAll(toPrettySentence(f.Usage), "|", `&#124;`)
		_, _ = fmt.Fprintf(w, "| %s | %s | %s |\n", f.Name, f.DefValue, desc)
	})
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
