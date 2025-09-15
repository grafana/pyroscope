package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	gprofile "github.com/google/pprof/profile"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
)

type Config struct {
	targetURL string
	dryRun    bool
}

func main() {
	var cfg Config
	flag.StringVar(&cfg.targetURL, "url", "http://localhost:4040", "Target URL to send requests to")
	flag.BoolVar(&cfg.dryRun, "dry-run", false, "Do not send requests, just print them")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Error: at least one file or directory path is required\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] <path1> [path2] ...\n", os.Args[0])
		flag.Usage()
		os.Exit(1)
	}

	targetURL, err := url.Parse(cfg.targetURL)
	if err != nil {
		log.Fatalf("Invalid target URL: %v", err)
	}

	var allFiles []string
	for _, path := range args {
		files, err := getRequestFiles(path)
		if err != nil {
			log.Fatalf("Error getting request files from %s: %v", path, err)
		}
		allFiles = append(allFiles, files...)
	}

	if len(allFiles) == 0 {
		log.Fatal("No request files found")
	}

	allFiles = removeDuplicates(allFiles)

	fmt.Printf("Found %d request files to replay\n", len(allFiles))
	fmt.Printf("Target URL: %s\n", cfg.targetURL)

	replayer := &Replayer{
		targetURL: targetURL,
		dryRun:    cfg.dryRun,
	}

	if err := replayer.ReplayFiles(allFiles); err != nil {
		log.Fatalf("Error replaying requests: %v", err)
	}
	fmt.Println(cfg.dryRun)
}

func removeDuplicates(files []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, file := range files {
		absPath, err := filepath.Abs(file)
		if err != nil {
			absPath = file
		}
		if !seen[absPath] {
			seen[absPath] = true
			result = append(result, file)
		}
	}
	return result
}

func getRequestFiles(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	var files []string
	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".bin") {
				files = append(files, filepath.Join(path, entry.Name()))
			}
		}
	} else {
		files = []string{path}
	}

	return files, nil
}

type Replayer struct {
	targetURL *url.URL
	dryRun    bool
}

func (r *Replayer) ReplayFiles(files []string) error {
	startTime := time.Now()
	successCount := 0
	failCount := 0

	for _, file := range files {
		if err := r.replayFile(file); err != nil {
			failCount++
			log.Printf("Error replaying %s: %v", file, err)
		} else {
			successCount++
			log.Printf("Successfully replayed %s", file)
		}
	}

	duration := time.Since(startTime)
	fmt.Printf("\nReplay completed in %s\n", duration)
	fmt.Printf("Success: %d, Failed: %d\n", successCount, failCount)

	return nil
}

func (r *Replayer) replayFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	req, err := http.ReadRequest(bufio.NewReader(file))
	if err != nil {
		return fmt.Errorf("reading request: %w", err)
	}

	newURL := *r.targetURL
	newURL.Path = req.URL.Path
	newURL.RawQuery = req.URL.RawQuery

	newReq, err := http.NewRequest(req.Method, newURL.String(), req.Body)
	if err != nil {
		return fmt.Errorf("creating new request: %w", err)
	}

	for key, values := range req.Header {
		if key == "Host" {
			continue
		}
		for _, value := range values {
			newReq.Header.Add(key, value)
		}
	}

	if r.dryRun {
		body, _ := io.ReadAll(req.Body)

		if req.URL.Path == "/push.v1.PusherService/Push" {
			var pushReq pushv1.PushRequest
			if err := proto.Unmarshal(body, &pushReq); err != nil {
				fmt.Printf("[%s] %s %s %d bytes (failed to deserialize: %v)\n",
					filepath.Base(filePath), req.Method, req.URL.Path, len(body), err)
			} else {
				fmt.Printf("[%s] %s %s %d bytes\n",
					filepath.Base(filePath), req.Method, req.URL.Path, len(body))
				fmt.Printf("  Series count: %d\n", len(pushReq.Series))
				for i, series := range pushReq.Series {
					fmt.Printf("  Series[%d]: %d labels, %d samples\n",
						i, len(series.Labels), len(series.Samples))
					if len(series.Labels) > 0 {
						fmt.Printf("    Labels: ")
						for j, label := range series.Labels {
							if j > 0 {
								fmt.Printf(", ")
							}
							fmt.Printf("%s=%s", label.Name, label.Value)
						}
						fmt.Printf("\n")
					}
					for _, s := range series.Samples {
						p, err := gprofile.ParseData(s.RawProfile)
						if err != nil {
							fmt.Printf("    Sample[%d]: %d bytes (failed to deserialize: %v)\n",
								i, len(s.RawProfile), err)
						} else {
							fmt.Println(p.String())
						}
					}
				}
			}
		} else {
			fmt.Printf("[%s] %s %s %d bytes\n", filepath.Base(filePath), req.Method, req.URL.Path, len(body))
		}
		return nil
	}
	fmt.Println("=================")
	resp, err := http.DefaultClient.Do(newReq)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	io.Copy(io.Discard, resp.Body)

	fmt.Printf("[%s] %s %s -> %d\n", filepath.Base(filePath), req.Method, req.URL.Path, resp.StatusCode)

	return nil
}
