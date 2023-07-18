package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func usage() {
	f, w := flag.CommandLine, flag.CommandLine.Output()
	fmt.Fprintf(w, `Decode response from pyroscope, for debugging

Usage: decode-resp -file <response.json>
  where <response.json> is the body response from /render API
  example: http://localhost:4040/render?from=now-1h&until=now&name=pyroscope.server.alloc_objects{}&max-nodes=1024&format=json

Flags:
`)
	f.PrintDefaults()
}

func main() {
	var inFile string
	var outFile string

	flag.StringVar(&inFile, "file", "", "path to response file (required)")
	flag.StringVar(&outFile, "out", "", "name of output file (default to NAME.out.EXT)")
	flag.Usage = usage
	flag.Parse()

	if inFile == "" {
		usage()
		os.Exit(2)
	}
	if outFile == "" {
		dir, base := filepath.Dir(inFile), filepath.Base(inFile)
		ext := filepath.Ext(base)
		name := base[:len(base)-len(ext)]
		outFile = filepath.Join(dir, name+".out"+ext)
	}

	// read file
	inData, err := os.ReadFile(inFile)
	must(err)
	var input Input
	must(json.Unmarshal(inData, &input))

	// decode
	output := decodeLevels(&input)

	// write file
	outData, err := json.MarshalIndent(output, "", "  ")
	must(err)
	must(os.WriteFile(outFile, outData, 0644))

	fmt.Fprintf(os.Stderr, "decoded to %v\n", outFile)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
