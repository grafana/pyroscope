package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	log2 "github.com/go-kit/log"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/pkg/experiment/symbolizer"
)

const (
	// buildID           = "c047672cae7964324658491e7dee26748ae5d2f8"
	//buildID = "2fa2055ef20fabc972d5751147e093275514b142"
	buildID     = "6d64b17fbac799e68da7ebd9985ddf9b5cb375e6"
	invalidName = "<invalid>"
)

func main() {
	client := &localDebuginfodClient{debugFilePath: "/usr/lib/debug/.build-id/6d/64b17fbac799e68da7ebd9985ddf9b5cb375e6.debug"}
	logger := log2.NewLogfmtLogger(os.Stdout)
	s, err := symbolizer.NewProfileSymbolizer(logger, client, nil, symbolizer.NewMetrics(nil), 10, 10, nil)

	if err != nil {
		log.Fatalf("Failed to create debuginfod client: %v", err)
	}

	ctx := context.Background()
	_, err = client.FetchDebuginfo(ctx, buildID)
	if err != nil {
		log.Fatalf("Failed to fetch debug info: %v", err)
	}

	profile := &googlev1.Profile{
		Mapping: []*googlev1.Mapping{{
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
			},
			{
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

	fmt.Println("\nFunction Table:")
	for i, fn := range p.Function {
		name := invalidName
		if fn.Name >= 0 && int(fn.Name) < len(p.StringTable) {
			name = p.StringTable[fn.Name]
		}

		filename := invalidName
		if fn.Filename >= 0 && int(fn.Filename) < len(p.StringTable) {
			filename = p.StringTable[fn.Filename]
		}

		fmt.Printf("  Function[%d]: ID=%d, Name=%s, File=%s, StartLine=%d\n",
			i, fn.Id, name, filename, fn.StartLine)
	}

	fmt.Println("\nLocations:")
	for _, loc := range p.Location {
		fmt.Printf("\nAddress: 0x%x\n", loc.Address)
		if len(loc.Line) == 0 {
			fmt.Println("  No symbolization information found")
			continue
		}

		for i, line := range loc.Line {
			fmt.Printf("  Line %d (FunctionID=%d):\n", i+1, line.FunctionId)

			var fn *googlev1.Function
			for _, function := range p.Function {
				if function.Id == line.FunctionId {
					fn = function
					break
				}
			}

			if fn != nil {
				name := invalidName
				if fn.Name >= 0 && int(fn.Name) < len(p.StringTable) {
					name = p.StringTable[fn.Name]
				}

				filename := invalidName
				if fn.Filename >= 0 && int(fn.Filename) < len(p.StringTable) {
					filename = p.StringTable[fn.Filename]
				}

				fmt.Printf("    Function:   %s\n", name)
				fmt.Printf("    File:       %s\n", filename)
				fmt.Printf("    Line:       %d\n", line.Line)
				fmt.Printf("    StartLine:  %d\n", fn.StartLine)
			} else {
				fmt.Printf("    ERROR: Cannot find function with ID %d\n", line.FunctionId)
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
