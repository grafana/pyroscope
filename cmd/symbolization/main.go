package main

import (
	"context"
	"fmt"
	"log"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/pkg/experiment/symbolizer"
)

const (
	debuginfodBaseURL = "https://debuginfod.elfutils.org"
	// buildID           = "c047672cae7964324658491e7dee26748ae5d2f8"
	buildID = "2fa2055ef20fabc972d5751147e093275514b142"
)

func main() {
	client := symbolizer.NewDebuginfodClient(debuginfodBaseURL, symbolizer.NewMetrics(nil))

	// Alternatively, use a local debug info file:
	//client := &localDebuginfodClient{debugFilePath: "/path/to/your/debug/file"}

	s := symbolizer.NewProfileSymbolizer(client, nil, symbolizer.NewMetrics(nil))
	ctx := context.Background()

	_, err := client.FetchDebuginfo(ctx, buildID)
	if err != nil {
		log.Fatalf("Failed to fetch debug info: %v", err)
	}

	// {
	// 	Address: 0x1500,
	// 	Mapping: &pprof.Mapping{},
	// },
	// {
	// 	Address: 0x3c5a,
	// 	Mapping: &pprof.Mapping{},
	// },
	// {
	// 	Address: 0x2745,
	// 	Mapping: &pprof.Mapping{},
	// },

	// Create a profile with the address we want to symbolize
	// c047672cae7964324658491e7dee26748ae5d2f8
	// profile := &googlev1.Profile{
	// 	Mapping: []*googlev1.Mapping{{
	// 		BuildId:     1,
	// 		MemoryStart: 0x0,
	// 		MemoryLimit: 0x1000000,
	// 		FileOffset:  0x0,
	// 	}},
	// 	Location: []*googlev1.Location{{
	// 		MappingId: 1,
	// 		Address:   0x11a230,
	// 	}},
	// 	StringTable: []string{"", buildID},
	// }

	profile := &googlev1.Profile{
		Mapping: []*googlev1.Mapping{{
			BuildId:     1,
			MemoryStart: 0x0,
			MemoryLimit: 0x1000000,
			FileOffset:  0x0,
		}},
		Location: []*googlev1.Location{
			{
				MappingId: 1,
				Address:   0x1500,
			},
			{
				MappingId: 1,
				Address:   0x3c5a,
			},
			{
				MappingId: 1,
				Address:   0x2745,
			},
		},
		StringTable: []string{"", buildID},
	}

	// Run symbolization
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

// localDebuginfodClient provides a way to use local debug info files instead of fetching from a server
//
//nolint:all
type localDebuginfodClient struct {
	debugFilePath string
}

//nolint:all
func (c *localDebuginfodClient) FetchDebuginfo(buildID string) (string, error) {
	return c.debugFilePath, nil
}
