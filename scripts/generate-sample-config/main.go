package main

import (
	"flag"
	"fmt"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/sirupsen/logrus"
)

// to run this program:
//   go run scripts/generate-sample-config/main.go -format yaml
//   go run scripts/generate-sample-config/main.go -format md

func main() {
	var format string
	flag.StringVar(&format, "format", "yaml", "yaml or md")
	flag.Parse()

	serverFlagSet := flag.NewFlagSet("pyroscope server", flag.ExitOnError)
	cfg := config.Config{Server: config.Server{}}
	cli.PopulateFlagSet(&cfg.Server, serverFlagSet)

	sf := cli.NewSortedFlags(&cfg.Server, serverFlagSet)

	if format == "yaml" {
		fmt.Println("---")
		sf.VisitAll(func(f *flag.Flag) {
			if f.Name != "config" {
				fmt.Printf("# %s\n%s: %q\n\n", f.Usage, f.Name, f.DefValue)
			}
		})
	} else if format == "md" {
		fmt.Printf("| %s | %s | %s |\n", "Name", "Default Value", "Usage")
		fmt.Printf("| %s | %s | %s |\n", ":-", ":-", ":-")
		sf.VisitAll(func(f *flag.Flag) {
			if f.Name != "config" {
				fmt.Printf("| %s | %s | %q |\n", f.Name, f.DefValue, f.Usage)
			}
		})
	} else {
		logrus.Fatalf("Unknown format %q", format)
	}
}
