package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/grafana/pyroscope/pkg/frontend/vcs/config"
)

func main() {
	var (
		jarPath    = flag.String("jar", "", "Path to the Java JAR file to analyze")
		configPath = flag.String("config", "", "Path to existing .pyroscope.yaml file to modify (default: print complete config to stdout)")
		jdkVersion = flag.String("jdk-version", "", "JDK version for JDK function mappings (e.g., '8', '11', '17', '21'). If not specified, will attempt to extract from JAR file.")
		help       = flag.Bool("help", false, "Show help")
	)
	flag.Parse()

	if *help || *jarPath == "" {
		fmt.Println("Java JAR Source Code Mapper")
		fmt.Println()
		fmt.Println("Generates .pyroscope.yaml source_code mappings for 3rd party libraries")
		fmt.Println("found in a Java JAR file.")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Printf("  %s -jar <jar-file> [-config <config-file>] [-jdk-version <version>]\n", os.Args[0])
		fmt.Println()
		fmt.Println("Flags:")
		flag.PrintDefaults()
		return
	}

	httpClient := NewHTTPClient()
	depsDevService := NewDepsDevService(httpClient)
	githubTagService := NewGitHubTagService(httpClient)
	configService := &ConfigService{}

	_, err := configService.LoadJarMappings()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load JAR mappings: %v\n", err)
	}

	githubResolver := NewGitHubResolver(depsDevService, configService)

	jarAnalyzer := NewJARAnalyzer()
	jarExtractor := &JARExtractor{}

	processor := NewProcessor(
		jarAnalyzer,
		githubResolver,
		configService,
		githubTagService,
	)

	mappingService := NewMappingService(processor, jarExtractor)

	// Process the JAR file
	mappings, err := mappingService.ProcessJAR(*jarPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Generate JDK mappings
	var jdkMappings []config.MappingConfig
	jdkVersionToUse := *jdkVersion
	if jdkVersionToUse == "" {
		extractedVersion, err := extractJDKVersionFromJAR(*jarPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not extract JDK version from JAR: %v\n", err)
			fmt.Fprintf(os.Stderr, "Skipping JDK mappings. Use -jdk-version flag to specify JDK version manually.\n")
		} else {
			jdkVersionToUse = extractedVersion
			fmt.Fprintf(os.Stderr, "Detected JDK version: %s\n", jdkVersionToUse)
		}
	}
	if jdkVersionToUse != "" {
		jdkMappings = generateJDKMappings(jdkVersionToUse)
	}

	if err := GenerateOrMergeConfig(*configPath, mappings, jdkMappings); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
