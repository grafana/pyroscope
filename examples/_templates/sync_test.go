package templates_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

const (
	modeCheck  = "check"
	modeUpdate = "update"

	// templatesDir is the repo-relative path of the templates directory.
	// It is used solely to build the "generated from" path in file headers;
	// update it here if the directory is ever moved.
	templatesDir = "examples/_templates"
)

// checkMode returns the value of EXAMPLE_CHECK_MODE ("check" or "update").
// Any other value (including unset) causes the test to be skipped.
func checkMode() string { return os.Getenv("EXAMPLE_CHECK_MODE") }

// fileHeader returns the "do not edit" comment block prepended to every
// non-docker-compose template file (Style A: #-comments at column 0).
// tmplName is the template directory name (e.g. "grafana"); rel is the
// file path relative to that template directory.
func fileHeader(tmplName, rel string) string {
	src := templatesDir + "/" + tmplName + "/" + rel
	return "# This file is generated from the template:\n" +
		"# " + src + "\n" +
		"# Do not edit directly. To update, edit the template and run:\n" +
		"#   make examples/sync-templates\n"
}

// serviceHeader returns the two-line "do not edit" comment injected inside a
// docker-compose service block (Style B: 4-space-indented #-comments).
// tmplName is the template directory name (e.g. "grafana").
func serviceHeader(tmplName string) string {
	src := templatesDir + "/" + tmplName + "/docker-compose.yml"
	return "    # This service is generated from " + src + "\n" +
		"    # Do not edit directly. To update, edit the template and run: make examples/sync-templates\n"
}

// injectServiceHeader inserts the Style B header comment on the second line of
// a service block (right after the "  <service>:" key line).
func injectServiceHeader(block, tmplName string) string {
	idx := strings.Index(block, "\n")
	if idx < 0 {
		return block
	}
	return block[:idx+1] + serviceHeader(tmplName) + block[idx+1:]
}

// example registers one example directory and which templates apply to it.
type example struct {
	// compose is the path to the folder containing the docker-compose file,
	// relative to examples/.
	compose string
	// templates lists template directory names (under examples/templates/) that
	// govern this example. For each template:
	//   - docker-compose.yml → the service it defines is checked structurally.
	//   - all other files    → checked byte-for-byte at the same relative path
	//                          inside the example directory.
	templates []string
	// serviceVolumes maps a service name to extra volume mount lines that are
	// appended after the template's own volumes entries for that service.
	// Use this when an example legitimately extends a templated service with
	// additional volume mounts (e.g. a custom grafana.ini or home dashboard).
	// The template prefix is still enforced; only the extra lines are allowed
	// to deviate. Lines should use the same 4-space indent as the template
	// (e.g. "    - ./grafana/grafana.ini:/etc/grafana/grafana.ini").
	serviceVolumes map[string][]string
}

// j is a shorthand for filepath.Join.
var j = filepath.Join

// examples is the authoritative list of every example directory and which
// templates apply to it. When adding a new example, register it here.
var examples = []example{
	// ── tracing ──────────────────────────────────────────────────────────────
	{
		compose:   "tracing/dotnet",
		templates: []string{"grafana", "tempo", "pyroscope"},
	},
	{
		compose:   "tracing/golang-push",
		templates: []string{"grafana", "tempo", "pyroscope"},
	},
	{
		compose:   "tracing/java",
		templates: []string{"grafana", "tempo", "pyroscope"},
	},
	{
		compose:   "tracing/java-wall",
		templates: []string{"grafana", "tempo", "pyroscope"},
	},
	{
		compose:   "tracing/python",
		templates: []string{"grafana", "tempo", "pyroscope"},
	},
	{
		compose:   "tracing/ruby",
		templates: []string{"grafana", "tempo", "pyroscope"},
	},
	{
		compose:   "tracing/tempo",
		templates: []string{"grafana", "tempo", "pyroscope"},
	},

	// ── base-url ──────────────────────────────────────────────────────────────
	{
		compose: "base-url",
		// grafana absent; pyroscope uses a custom command flag
	},

	// ── golang-pgo ───────────────────────────────────────────────────────────
	{
		compose:   "golang-pgo",
		templates: []string{"grafana", "pyroscope"},
	},

	// ── grafana-alloy-auto-instrumentation ────────────────────────────────────
	{
		compose:   "grafana-alloy-auto-instrumentation/ebpf/docker",
		templates: []string{"grafana", "pyroscope"},
	},
	{
		compose:   "grafana-alloy-auto-instrumentation/ebpf/local",
		templates: []string{"grafana", "pyroscope"},
	},
	{
		// The grafana service mounts extra files (grafana.ini, home.json) for a
		// custom default home dashboard — tracked via serviceVolumes so the
		// template's shared config is still enforced.
		compose:   "grafana-alloy-auto-instrumentation/golang-pull",
		templates: []string{"grafana", "pyroscope"},
		serviceVolumes: map[string][]string{
			"grafana": {
				"    - ./grafana/grafana.ini:/etc/grafana/grafana.ini",
				"    - ./grafana/home.json:/default-dashboard.json",
			},
		},
	},
	{
		compose:   "grafana-alloy-auto-instrumentation/java/docker",
		templates: []string{"grafana", "pyroscope"},
	},

	// ── language-sdk-instrumentation/dotnet ──────────────────────────────────
	{
		compose:   "language-sdk-instrumentation/dotnet/fast-slow",
		templates: []string{"grafana", "pyroscope"},
	},
	{
		compose:   "language-sdk-instrumentation/dotnet/rideshare",
		templates: []string{"grafana", "pyroscope"},
	},
	{
		compose:   "language-sdk-instrumentation/dotnet/web-new",
		templates: []string{"grafana", "pyroscope"},
	},

	// ── language-sdk-instrumentation/golang-push ─────────────────────────────
	{
		// The rideshare example also provisions a Prometheus datasource because
		// its app exports OTLP metrics to Prometheus. The prometheus template
		// provides the service definition and grafana datasource provisioning.
		compose:   "language-sdk-instrumentation/golang-push/rideshare",
		templates: []string{"grafana", "pyroscope", "prometheus"},
	},
	{
		compose:   "language-sdk-instrumentation/golang-push/rideshare-alloy",
		templates: []string{"grafana", "pyroscope"},
	},
	{
		compose:   "language-sdk-instrumentation/golang-push/rideshare-k6",
		templates: []string{"grafana", "pyroscope"},
	},
	{
		compose:   "language-sdk-instrumentation/golang-push/simple",
		templates: []string{"grafana", "pyroscope"},
	},

	// ── language-sdk-instrumentation/java ────────────────────────────────────
	{
		compose:   "language-sdk-instrumentation/java/fib",
		templates: []string{"grafana", "pyroscope"},
	},
	{
		compose:   "language-sdk-instrumentation/java/rideshare",
		templates: []string{"grafana", "pyroscope"},
	},
	{
		compose:   "language-sdk-instrumentation/java/simple",
		templates: []string{"grafana", "pyroscope"},
	},

	// ── language-sdk-instrumentation/nodejs ──────────────────────────────────
	{
		compose:   "language-sdk-instrumentation/nodejs/express",
		templates: []string{"grafana", "pyroscope"},
	},
	{
		compose:   "language-sdk-instrumentation/nodejs/express-pull",
		templates: []string{"grafana", "pyroscope"},
	},
	{
		compose:   "language-sdk-instrumentation/nodejs/express-ts",
		templates: []string{"grafana", "pyroscope"},
	},
	{
		compose:   "language-sdk-instrumentation/nodejs/express-ts-inline",
		templates: []string{"grafana", "pyroscope"},
	},
	{
		compose:   "language-sdk-instrumentation/nodejs/tinyhttp",
		templates: []string{"grafana", "pyroscope"},
	},

	// ── language-sdk-instrumentation/python ──────────────────────────────────
	{
		compose:   "language-sdk-instrumentation/python/rideshare/django",
		templates: []string{"grafana", "pyroscope"},
	},
	{
		compose:   "language-sdk-instrumentation/python/rideshare/fastapi",
		templates: []string{"grafana", "pyroscope"},
	},
	{
		compose:   "language-sdk-instrumentation/python/rideshare/flask",
		templates: []string{"grafana", "pyroscope"},
	},
	{
		compose:   "language-sdk-instrumentation/python/simple",
		templates: []string{"grafana", "pyroscope"},
	},

	// ── language-sdk-instrumentation/ruby ────────────────────────────────────
	{
		compose:   "language-sdk-instrumentation/ruby/rideshare",
		templates: []string{"grafana", "pyroscope"},
	},
	{
		compose:   "language-sdk-instrumentation/ruby/rideshare_rails",
		templates: []string{"grafana", "pyroscope"},
	},
	{
		compose:   "language-sdk-instrumentation/ruby/simple",
		templates: []string{"grafana", "pyroscope"},
	},

	// ── language-sdk-instrumentation/rust ────────────────────────────────────
	{
		compose:   "language-sdk-instrumentation/rust/basic",
		templates: []string{"grafana", "pyroscope"},
	},
	{
		compose:   "language-sdk-instrumentation/rust/rideshare",
		templates: []string{"grafana", "pyroscope"},
	},

	// ── otel-collector ───────────────────────────────────────────────────────
	{
		compose: "otel-collector/ebpf/docker",
		// grafana omitted: has networks key
		// pyroscope omitted: pinned version + networks
	},
}

// TestExamplesConsistency verifies (or updates) every registered example's
// service blocks and config files against their templates.
//
// Check: EXAMPLE_CHECK_MODE=check go test ./examples/templates/
// Sync:  EXAMPLE_CHECK_MODE=update go test ./examples/templates/
func TestExamplesConsistency(t *testing.T) {
	switch checkMode() {
	case modeCheck, modeUpdate:
		// proceed
	default:
		t.Skip("set EXAMPLE_CHECK_MODE=check or EXAMPLE_CHECK_MODE=update to run")
	}

	for _, ex := range examples {
		ex := ex
		t.Run(ex.compose, func(t *testing.T) {
			exDir := j("..", ex.compose)
			composePath := findCompose(t, exDir)

			for _, tmplName := range ex.templates {
				tmplName := tmplName
				t.Run(tmplName, func(t *testing.T) {
					tmplDir := tmplName

					// Walk everything in the template dir.
					err := filepath.WalkDir(tmplDir, func(path string, d fs.DirEntry, err error) error {
						require.NoError(t, err)
						if d.IsDir() {
							return nil
						}

						rel, err := filepath.Rel(tmplDir, path)
						require.NoError(t, err)

						if rel == "docker-compose.yml" {
							// Service block check: structural YAML comparison.
							svcName, canonical := loadTemplateService(t, path)
							// Inject the "do not edit" header comment into the block.
							canonical = injectServiceHeader(canonical, tmplName)
							// Apply any per-example extra volume mounts.
							if extra, ok := ex.serviceVolumes[svcName]; ok {
								canonical = injectVolumes(t, canonical, extra)
							}
							t.Run(svcName, func(t *testing.T) {
								if checkMode() == modeUpdate {
									require.NoError(t, updateService(t, composePath, svcName, canonical))
									return
								}
								got, err := extractService(t, composePath, svcName)
								require.NoError(t, err)
								require.NotEmpty(t, got, "service %q not found in %s", svcName, composePath)
								require.Equal(t, canonical, got,
									"services.%s in %s differs from template %s\n"+
										"Update the template or fix the compose file.",
									svcName, composePath, path)
							})
						} else {
							// Config file check: prepend generated header then byte-identical.
							dstPath := j(exDir, rel)
							t.Run(rel, func(t *testing.T) {
								body, err := os.ReadFile(path)
								require.NoError(t, err)
								canonical := []byte(fileHeader(tmplName, rel))
								canonical = append(canonical, body...)
								if checkMode() == modeUpdate {
									require.NoError(t, os.MkdirAll(filepath.Dir(dstPath), 0o755))
									require.NoError(t, os.WriteFile(dstPath, canonical, 0o644))
									return
								}
								got, err := os.ReadFile(dstPath)
								require.NoError(t, err)
								require.Equal(t, string(canonical), string(got),
									"%s differs from template %s\nRun: EXAMPLE_CHECK_MODE=update go test ./examples/templates/",
									dstPath, path)
							})
						}
						return nil
					})
					require.NoError(t, err)
				})
			}
		})
	}
}

// injectVolumes appends extra volume mount lines to the volumes: block of a
// service block string. The extra lines must use the same indentation as the
// existing entries (4-space indent). The volumes: key must already exist in
// the block.
func injectVolumes(t *testing.T, block string, extra []string) string {
	t.Helper()
	lines := strings.Split(block, "\n")
	// Find the last existing volume entry — extra lines go right after it.
	insertAt := -1
	inVolumes := false
	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "volumes:" {
			inVolumes = true
			continue
		}
		if inVolumes {
			if strings.HasPrefix(l, "    -") {
				insertAt = i
			} else if trimmed != "" && !strings.HasPrefix(l, "    -") {
				// Hit a new key at the same or higher level: stop.
				break
			}
		}
	}
	require.NotEqual(t, -1, insertAt, "injectVolumes: no volume entries found in service block")
	result := make([]string, 0, len(lines)+len(extra))
	result = append(result, lines[:insertAt+1]...)
	result = append(result, extra...)
	result = append(result, lines[insertAt+1:]...)
	return strings.Join(result, "\n")
}

// findCompose returns the path to the docker-compose file inside dir.
func findCompose(t *testing.T, dir string) string {
	t.Helper()
	for _, name := range []string{"docker-compose.yml", "docker-compose.yaml"} {
		p := j(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Fatalf("no docker-compose file found in %s", dir)
	return ""
}

// loadTemplateService reads a template docker-compose.yml, asserts it has
// exactly one service, and returns the service name and its raw text block.
func loadTemplateService(t *testing.T, path string) (name, content string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var root yaml.Node
	require.NoError(t, yaml.Unmarshal(data, &root))
	topMap := root.Content[0]
	servicesNode := mappingValue(topMap, "services")
	require.NotNil(t, servicesNode, "%s: missing 'services' key", path)
	require.Equal(t, 2, len(servicesNode.Content), "%s: template must define exactly one service", path)
	name = servicesNode.Content[0].Value
	content, err = serviceBlock(data, topMap, name)
	require.NoError(t, err)
	return name, content
}

// extractService returns the raw text block of the named service from a
// docker-compose file, or an empty string if the service is not found.
func extractService(t *testing.T, path, svcName string) (string, error) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return "", err
	}
	if len(root.Content) == 0 {
		return "", nil
	}
	return serviceBlock(data, root.Content[0], svcName)
}

// updateService replaces the named service block in a docker-compose file with
// the provided canonical block (which already includes any per-example extras).
func updateService(t *testing.T, composePath, svcName, canonical string) error {
	t.Helper()

	composeData, err := os.ReadFile(composePath)
	if err != nil {
		return err
	}
	var composeRoot yaml.Node
	if err := yaml.Unmarshal(composeData, &composeRoot); err != nil {
		return err
	}
	s, e := serviceLines(composeData, composeRoot.Content[0], svcName)
	if s < 0 {
		return fmt.Errorf("service %q not found in %s", svcName, composePath)
	}

	lines := strings.Split(string(composeData), "\n")
	var out strings.Builder
	for _, l := range lines[:s] {
		out.WriteString(l)
		out.WriteByte('\n')
	}
	out.WriteString(canonical)
	for _, l := range lines[e:] {
		out.WriteString(l)
		out.WriteByte('\n')
	}
	result := strings.TrimRight(out.String(), "\n") + "\n"
	return os.WriteFile(composePath, []byte(result), 0o644)
}

// serviceBlock returns the raw source text of the named service block,
// using yaml.Node line numbers to slice the file — no marshalling.
func serviceBlock(data []byte, topMap *yaml.Node, svcName string) (string, error) {
	s, e := serviceLines(data, topMap, svcName)
	if s < 0 {
		return "", nil
	}
	lines := strings.Split(string(data), "\n")
	return strings.Join(lines[s:e], "\n") + "\n", nil
}

// serviceLines returns the 0-indexed [start, end) line range of the named
// service block, using yaml.Node.Line for boundaries. end trims trailing blank
// lines. Returns -1, -1 if the service is not found.
func serviceLines(data []byte, topMap *yaml.Node, svcName string) (start, end int) {
	servicesNode := mappingValue(topMap, "services")
	if servicesNode == nil {
		return -1, -1
	}

	var startLine, endLine int // 1-indexed; endLine==0 means "not yet known"
	for i := 0; i < len(servicesNode.Content)-1; i += 2 {
		if servicesNode.Content[i].Value == svcName {
			startLine = servicesNode.Content[i].Line
			if i+2 < len(servicesNode.Content) {
				// Next sibling service key marks the end.
				endLine = servicesNode.Content[i+2].Line
			}
			break
		}
	}
	if startLine == 0 {
		return -1, -1
	}

	lines := strings.Split(string(data), "\n")
	if endLine == 0 {
		// Last (or only) service: end at the next top-level key, or EOF.
		for i := 0; i < len(topMap.Content)-1; i += 2 {
			if topMap.Content[i].Value == "services" && i+2 < len(topMap.Content) {
				endLine = topMap.Content[i+2].Line
				break
			}
		}
		if endLine == 0 {
			endLine = len(lines) + 1
		}
	}

	// Convert to 0-indexed and trim trailing blank lines.
	s := startLine - 1
	e := endLine - 1
	for e > s && strings.TrimSpace(lines[e-1]) == "" {
		e--
	}
	return s, e
}

// mappingValue returns the value node for key in a YAML MappingNode, or nil.
func mappingValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i < len(m.Content)-1; i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}
