package main

import (
	"context"
	"fmt"
	log2 "github.com/go-kit/log"
	"io"
	"log"
	"os"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/pkg/experiment/symbolizer"
)

const (
	buildID = "6d64b17fbac799e68da7ebd9985ddf9b5cb375e6"
)

func main() {

	client := &localDebuginfodClient{debugFilePath: "/usr/lib/debug/.build-id/6d/64b17fbac799e68da7ebd9985ddf9b5cb375e6.debug"}
	logger := log2.NewLogfmtLogger(os.Stdout)
	s, err := symbolizer.NewProfileSymbolizer(logger, client, nil, symbolizer.NewMetrics(nil), 10, 10)
	if err != nil {
		log.Fatalf("Failed to create symbolizer: %v", err)
	}

	ctx := context.Background()

	_, err = client.FetchDebuginfo(ctx, buildID)
	if err != nil {
		log.Fatalf("Failed to fetch debug info: %v", err)
	}

	profile := &googlev1.Profile{
		Mapping: []*googlev1.Mapping{{
			BuildId:     1,
			MemoryStart: 0x28000,
			MemoryLimit: 0x1b0000,
			FileOffset:  0x28000,
		}},
		Location: []*googlev1.Location{
			{
				MappingId: 1,
				Address:   0x2a28a,
			},
			{
				MappingId: 1,
				Address:   0x124dec,
			}, {
				MappingId: 1,
				Address:   0x2a1c9,
			},
		},
		StringTable: []string{"", buildID},
	}

	if err := s.SymbolizePprof(ctx, profile); err != nil {
		log.Fatalf("Failed to symbolize: %v", err)
	}

	printResults(profile)
}

func printResults(p *googlev1.Profile) {
	fmt.Println("Symbolization Results:")
	for _, loc := range p.Location {
		fmt.Printf("\nAddress: 0x%x\n", loc.Address)
		if len(loc.Line) == 0 {
			fmt.Println("  No symbolization information found")
			continue
		}

		for i, line := range loc.Line {
			fmt.Printf("  Line %d:\n", i+1)
			if fn := p.Function[line.FunctionId]; fn != nil {
				fmt.Printf("    Function:   %s\n", p.StringTable[fn.Name])
				fmt.Printf("    File:       %s\n", p.StringTable[fn.Filename])
				fmt.Printf("    Line:       %d\n", line.Line)
				fmt.Printf("    StartLine:  %d\n", fn.StartLine)
			}
		}
	}
	fmt.Println("\nSymbolization completed successfully.")
}

type localDebuginfodClient struct {
	debugFilePath string
}

func (c *localDebuginfodClient) FetchDebuginfo(ctx context.Context, buildID string) (io.ReadCloser, error) {
	return os.Open(c.debugFilePath)
}
