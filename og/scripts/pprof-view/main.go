package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/convert/pprof"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"

	"gopkg.in/yaml.v2"
)

func main() {
	// the idea here is that you can run it via go main like so:
	//   cat heap.pprof.gz | go run scripts/pprof-view/main.go
	// and the script will print a json version of a given profile
	if len(os.Args) == 1 {
		if err := dumpJSON(os.Stdout); err != nil {
			log.Fatalln(err)
		}
		return
	}

	// You can also parse pprof with a config:
	//   go run scripts/pprof-view/main.go -path heap.pb.gz -type mem -config ./scripts/pprof-view/pprof-config.yaml
	// If config is not specified, the default one is used (see tree.DefaultSampleTypeMapping).
	var (
		configPath  string
		pprofPath   string
		profileType string
	)

	flag.StringVar(&configPath, "config", "", "path to pprof parsing config")
	flag.StringVar(&pprofPath, "path", "", "path tp pprof data (gzip or plain)")
	flag.StringVar(&profileType, "type", "cpu", "profile type from the config (cpu, mem, goroutines, etc)")
	flag.Parse()

	if err := printProfiles(os.Stdout, pprofPath, configPath, profileType); err != nil {
		log.Fatalln(err)
	}
}

func dumpJSON(w io.Writer) error {
	var p tree.Profile
	if err := pprof.Decode(os.Stdin, &p); err != nil {
		return err
	}
	b, err := json.MarshalIndent(&p, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}

type ingester struct{ actual []*storage.PutInput }

func (m *ingester) Put(_ context.Context, p *storage.PutInput) error {
	m.actual = append(m.actual, p)
	return nil
}

func printProfiles(w io.Writer, pprofPath, configPath, profileType string) error {
	c := tree.DefaultSampleTypeMapping
	if configPath != "" {
		sc, err := readPprofConfig(configPath)
		if err != nil {
			return fmt.Errorf("reading pprof parsing config: %w", err)
		}
		var ok bool
		if c, ok = sc[profileType]; !ok {
			return fmt.Errorf("profile type not found in the config")
		}
	}

	p, err := readPprof(pprofPath)
	if err != nil {
		return fmt.Errorf("reading pprof file: %w", err)
	}

	x := new(ingester)
	pw := pprof.NewParser(pprof.ParserConfig{
		Putter:      x,
		SampleTypes: c,
		SpyName:     "spy-name",
		Labels:      nil,
	})

	if err = pw.Convert(context.TODO(), time.Time{}, time.Time{}, p, false); err != nil {
		return fmt.Errorf("parsing pprof: %w", err)
	}

	_, _ = fmt.Fprintln(w, "Found profiles:", len(x.actual))
	for i, profile := range x.actual {
		_, _ = fmt.Fprintln(w, strings.Repeat("-", 80))
		_, _ = fmt.Fprintf(w, "Profile %d: <app_name>%s\n", i+1, profile.Key.Normalized())
		_, _ = fmt.Fprintln(w, "\tAggregation:", profile.AggregationType)
		_, _ = fmt.Fprintln(w, "\tUnits:", profile.Units)
		_, _ = fmt.Fprintln(w, "\tTotal:", profile.Val.Samples())
		_, _ = fmt.Fprintln(w, "\tSample rate:", profile.SampleRate)
		_, _ = fmt.Fprintf(w, "\n%s\n", profile.Val)
	}

	return nil
}

func readPprofConfig(path string) (map[string]map[string]*tree.SampleTypeConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()
	var c map[string]map[string]*tree.SampleTypeConfig
	if err = yaml.NewDecoder(f).Decode(&c); err != nil {
		return nil, err
	}
	return c, nil
}

func readPprof(path string) (*tree.Profile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()
	var p tree.Profile
	if err = pprof.Decode(f, &p); err != nil {
		return nil, err
	}
	return &p, nil
}
