package main

import (
	"context"
	"fmt"
	"log"

	pprof "github.com/google/pprof/profile"
	"github.com/grafana/pyroscope/pkg/experiment/symbolization"
)

const (
	debuginfodBaseURL = "https://debuginfod.elfutils.org"
	buildID           = "2fa2055ef20fabc972d5751147e093275514b142"
)

func main() {
	client := symbolization.NewDebuginfodClient(debuginfodBaseURL)

	// Alternatively, use a local debug info file:
	//client := &localDebuginfodClient{debugFilePath: "/path/to/your/debug/file"}

	symbolizer := symbolization.NewSymbolizer(client)
	ctx := context.Background()

	_, err := client.FetchDebuginfo(buildID)
	if err != nil {
		log.Fatalf("Failed to fetch debug info: %v", err)
	}
	//defer os.Remove(debugFilePath)

	// Create a request to symbolize specific addresses
	req := symbolization.Request{
		BuildID: buildID,
		Mappings: []symbolization.RequestMapping{
			{
				Locations: []*symbolization.Location{
					{
						Address: 0x1500,
						Mapping: &pprof.Mapping{},
					},
					{
						Address: 0x3c5a,
						Mapping: &pprof.Mapping{},
					},
					{
						Address: 0x2745,
						Mapping: &pprof.Mapping{},
					},
				},
			},
		},
	}

	if err := symbolizer.Symbolize(ctx, req); err != nil {
		log.Fatalf("Failed to symbolize: %v", err)
	}

	fmt.Println("Symbolization Results:")
	fmt.Printf("Build ID: %s\n", buildID)
	fmt.Println("----------------------------------------")

	for i, mapping := range req.Mappings {
		fmt.Printf("Mapping #%d:\n", i+1)
		for _, loc := range mapping.Locations {
			fmt.Printf("\nAddress: 0x%x\n", loc.Address)
			if len(loc.Lines) == 0 {
				fmt.Println("  No symbolization information found")
				continue
			}

			for j, line := range loc.Lines {
				fmt.Printf("  Line %d:\n", j+1)
				if line.Function != nil {
					fmt.Printf("    Function:   %s\n", line.Function.Name)
					fmt.Printf("    File:       %s\n", line.Function.Filename)
					fmt.Printf("    Line:       %d\n", line.Line)
					fmt.Printf("    StartLine:  %d\n", line.Function.StartLine)
				} else {
					fmt.Println("    No function information available")
				}
			}
			fmt.Println("----------------------------------------")
		}
	}

	// Alternatively: Symbolize all addresses in the binary
	// Note: Comment out the above specific symbolization when using this
	// as it's a different approach meant for exploring all available symbols
	//if err := symbolizer.SymbolizeAll(ctx, buildID); err != nil {
	//	log.Fatalf("Failed to symbolize all addresses: %v", err)
	//}

	fmt.Println("\nSymbolization completed successfully.")
}

// localDebuginfodClient provides a way to use local debug info files instead of fetching from a server
type localDebuginfodClient struct {
	debugFilePath string
}

func (c *localDebuginfodClient) FetchDebuginfo(buildID string) (string, error) {
	return c.debugFilePath, nil
}
