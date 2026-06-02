//go:build examples

package examples

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"os/exec"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

const (
	timeoutPerExample = 20 * time.Minute
	// How long to wait for all containers to come up and stay running.
	runningTimeout = 90 * time.Second
	// How long to wait for profiles to be ingested and become queryable.
	profilesQueryTimeout = 3 * time.Minute
	// How long to wait for traces to be ingested and become searchable.
	tracesQueryTimeout = 3 * time.Minute
	pollInterval       = 3 * time.Second
)

const (
	pyroscopeService = "pyroscope"
	pyroscopePort    = "4040"
	tempoService     = "tempo"
	tempoPort        = "3200"
)

// activeStacks tracks the compose stacks that are currently up, so they can be
// torn down on an interrupt (where t.Cleanup does not run).
var (
	activeMu     sync.Mutex
	activeStacks = map[string]*env{}
)

func registerActive(e *env) { activeMu.Lock(); activeStacks[e.projectName()] = e; activeMu.Unlock() }
func unregisterActive(e *env) {
	activeMu.Lock()
	delete(activeStacks, e.projectName())
	activeMu.Unlock()
}

// TestMain installs a signal handler so that an interrupted run (Ctrl+C) still
// tears down the docker-compose stacks it started. t.Cleanup does not run when
// the test process is killed by a signal, which would otherwise leak containers.
func TestMain(m *testing.M) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		activeMu.Lock()
		envs := make([]*env, 0, len(activeStacks))
		for _, e := range activeStacks {
			envs = append(envs, e)
		}
		activeMu.Unlock()
		for _, e := range envs {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			_ = e.newCmd(ctx, "down", "--volumes").Run()
			cancel()
		}
		os.Exit(1)
	}()
	os.Exit(m.Run())
}

type env struct {
	dir  string // project dir of docker-compose, relative to the test working directory
	path string // path to docker-compose file
}

func (e *env) repoDir() string {
	return filepath.Join("examples", e.dir)
}

type status struct {
	Name  string `json:"Name"`
	State string `json:"State"`
}

func (e *env) projectName() string {
	h := sha256.New()
	_, _ = h.Write([]byte(e.dir))
	return fmt.Sprintf("%s_%x", filepath.Base(e.dir), h.Sum(nil)[0:2])
}

func (e *env) newCmd(ctx context.Context, args ...string) *exec.Cmd {
	c := exec.CommandContext(
		ctx,
		"docker",
		append([]string{
			"compose",
			"--file", e.path,
			"--project-directory", e.dir,
			"--project-name", e.projectName(),
		}, args...)...)
	return c
}

// run executes a docker compose command, buffering its output and surfacing it
// only when the command fails. This keeps successful runs readable under -v,
// while still capturing full logs for debugging failures.
func (e *env) run(t testing.TB, ctx context.Context, args ...string) error {
	out, err := e.newCmd(ctx, args...).CombinedOutput()
	if err != nil {
		t.Logf("$ docker compose %s\n%s", strings.Join(args, " "), string(out))
	}
	return err
}

func (e *env) containerStatus(ctx context.Context) ([]status, error) {
	data, err := e.newCmd(ctx, "ps", "--all", "--format", "json").Output()
	if err != nil {
		return nil, err
	}

	var stats []status
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		var s status
		err := dec.Decode(&s)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}

	return stats, nil
}

func (e *env) containersAllRunning(ctx context.Context) error {
	status, err := e.containerStatus(ctx)
	if err != nil {
		return err
	}
	if len(status) == 0 {
		return errors.New("no containers found")
	}

	var errs []error
	for _, s := range status {
		if s.State != "running" {
			errs = append(errs, fmt.Errorf("container %s is not running (state=%s)", s.Name, s.State))
		}
	}

	return errors.Join(errs...)
}

// prepareCompose rewrites the docker-compose file into a temporary copy so that:
//   - every published port is bound to a random host port on localhost only
//     (avoids conflicts when running examples in parallel and keeps the
//     unauthenticated test services off the runner's external interfaces), and
//   - the pyroscope (and, when present, tempo) services always publish their
//     API ports so the test can query them over localhost.
func (e *env) prepareCompose(t testing.TB) *env {
	var obj map[string]interface{}

	body, err := os.ReadFile(e.path)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(body, &obj))

	services, ok := obj["services"].(map[string]interface{})
	require.True(t, ok, "docker-compose file has no services map")

	for serviceName, service := range services {
		params, ok := service.(map[string]interface{})
		if !ok {
			require.NoError(t, fmt.Errorf("service '%s' is not a map", serviceName))
		}
		localhostBindPorts(params)
	}

	// Ensure the API ports we need to query are published (on localhost).
	pp, _ := strconv.Atoi(pyroscopePort)
	tp, _ := strconv.Atoi(tempoPort)
	if svc, ok := services[pyroscopeService].(map[string]interface{}); ok {
		ensurePublished(svc, pp)
	}
	if svc, ok := services[tempoService].(map[string]interface{}); ok {
		ensurePublished(svc, tp)
	}

	path := filepath.Join(t.TempDir(), "docker-compose.yml")
	data, err := yaml.Marshal(obj)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0644))

	return &env{dir: e.dir, path: path}
}

// containerPortOf returns the container-side port of a docker-compose ports
// entry, handling short ("3000", "3000:3000", "127.0.0.1:80:8080"), numeric
// (4040) and long-form ({target: 4040, ...}) syntaxes. Returns 0 if unknown.
func containerPortOf(p interface{}) int {
	switch v := p.(type) {
	case int:
		return v
	case string:
		parts := strings.Split(v, ":")
		n, _ := strconv.Atoi(parts[len(parts)-1])
		return n
	case map[string]interface{}:
		return containerPortOf(v["target"])
	default:
		return 0
	}
}

// localhostPort is a long-form ports entry that binds the container port to a
// random host port on the loopback interface only.
func localhostPort(containerPort int) map[string]any {
	return map[string]any{"target": containerPort, "host_ip": "127.0.0.1"}
}

// localhostBindPorts rewrites every published port of a service to a random
// loopback-only binding. This both avoids host port conflicts when examples run
// in parallel and prevents exposing services on external interfaces.
func localhostBindPorts(params map[string]interface{}) {
	ports, ok := params["ports"].([]interface{})
	if !ok {
		return
	}
	for i := range ports {
		if cp := containerPortOf(ports[i]); cp != 0 {
			ports[i] = localhostPort(cp)
		}
	}
}

func ensurePublished(params map[string]interface{}, containerPort int) {
	ports, _ := params["ports"].([]interface{})
	for _, p := range ports {
		if containerPortOf(p) == containerPort {
			return
		}
	}
	params["ports"] = append(ports, localhostPort(containerPort))
}

// hostPort resolves the random host port that the given service's container port
// has been published on.
func (e *env) hostPort(t testing.TB, ctx context.Context, service, containerPort string) string {
	out, err := e.newCmd(ctx, "port", service, containerPort).Output()
	require.NoError(t, err, "resolving host port for %s:%s", service, containerPort)
	line := strings.TrimSpace(string(out))
	require.NotEmpty(t, line, "no published host port for %s:%s", service, containerPort)
	line = strings.SplitN(line, "\n", 2)[0]
	idx := strings.LastIndex(line, ":")
	require.GreaterOrEqual(t, idx, 0, "unexpected docker compose port output: %q", line)
	return line[idx+1:]
}

// bringUp builds, pulls and starts the example detached.
func (e *env) bringUp(t *testing.T, ctx context.Context) {
	// Register before starting and tear down on cleanup, so the stack is removed
	// both on normal completion (t.Cleanup) and on interrupt (see TestMain).
	registerActive(e)
	t.Cleanup(func() {
		unregisterActive(e)
		if err := e.run(t, context.Background(), "down", "--volumes"); err != nil {
			t.Logf("cleanup error=%v", err)
		}
	})
	t.Run("build", func(t *testing.T) {
		require.NoError(t, e.run(t, ctx, "build"))
	})
	// pull first so containers can start immediately
	t.Run("pull", func(t *testing.T) {
		require.NoError(t, e.run(t, ctx, "pull"))
	})
	require.NoError(t, e.run(t, ctx, "up", "--detach"))
}

// waitRunning waits until all containers report a running state, failing the
// test if they don't within runningTimeout. A container that exits during this
// window (e.g. a crash on startup) is caught here.
func (e *env) waitRunning(t testing.TB, ctx context.Context) {
	poll(t, ctx, runningTimeout, func(progress func(string, ...any)) error {
		progress("[%s] waiting for all containers to be running...", e.repoDir())
		return e.containersAllRunning(ctx)
	})
}

// poll calls fn until it returns nil, the timeout elapses, or the context is
// cancelled. On timeout it fails the test with the last error fn returned.
//
// fn receives a progress reporter it should use to announce what it is doing.
// Each distinct message is logged once across the whole poll, so steps show up
// as they are first reached without spamming a line on every retry.
func poll(t testing.TB, ctx context.Context, timeout time.Duration, fn func(progress func(string, ...any)) error) {
	progress := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		t.Log(msg)
	}
	deadline := time.Now().Add(timeout)
	for {
		err := fn(progress)
		if err == nil {
			return
		}
		progress("waiting: %s", err.Error())
		if time.Now().After(deadline) {
			require.NoError(t, err, "condition not met within %s", timeout)
			return
		}
		select {
		case <-ctx.Done():
			require.NoError(t, ctx.Err())
			return
		case <-time.After(pollInterval):
		}
	}
}

// --- Pyroscope query helpers -------------------------------------------------

func nowWindowMillis() (start, end int64) {
	now := time.Now()
	return now.Add(-1 * time.Hour).UnixMilli(), now.Add(1 * time.Minute).UnixMilli()
}

func (e *env) pyroscopePost(ctx context.Context, host, apiPath string, reqBody any, out any) error {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("http://%s/%s", host, apiPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned %d: %s", apiPath, resp.StatusCode, string(data))
	}
	return json.Unmarshal(data, out)
}

type labelPair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type seriesResponse struct {
	LabelsSet []struct {
		Labels []labelPair `json:"labels"`
	} `json:"labelsSet"`
}

func labelValue(labels []labelPair, name string) string {
	for _, l := range labels {
		if l.Name == name {
			return l.Value
		}
	}
	return ""
}

// seriesData maps a service name to its ingested CPU/wall profile types. Those
// are the types span profiles are attached to; other types (memory, lock, etc.)
// are not useful for these checks and are dropped.
type seriesData map[string][]string

func sortedKeys(d seriesData) []string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// selfProfilingService is the service name Pyroscope uses for its own profiles.
// Examples that don't disable self-profiling will surface it; we exclude it so
// the checks validate the example's application rather than the server.
const selfProfilingService = "pyroscope"

// isCPUOrWallType reports whether a profile type is a CPU or wall-clock type.
func isCPUOrWallType(profileType string) bool {
	return strings.HasPrefix(profileType, "process_cpu:") || strings.HasPrefix(profileType, "wall:")
}

// discoverSeries queries the Series API and returns the application services and
// their ingested CPU/wall profile types. Self-profiling series and non-CPU/wall
// types are excluded.
func (e *env) discoverSeries(ctx context.Context, host string) (seriesData, error) {
	start, end := nowWindowMillis()
	var resp seriesResponse
	err := e.pyroscopePost(ctx, host, "querier.v1.QuerierService/Series", map[string]any{
		"start":      start,
		"end":        end,
		"labelNames": []string{"service_name", "__profile_type__"},
	}, &resp)
	if err != nil {
		return nil, err
	}

	data := seriesData{}
	for _, ls := range resp.LabelsSet {
		svc := labelValue(ls.Labels, "service_name")
		pt := labelValue(ls.Labels, "__profile_type__")
		if svc == "" || svc == selfProfilingService || !isCPUOrWallType(pt) {
			continue
		}
		data[svc] = append(data[svc], pt)
	}
	if len(data) == 0 {
		return nil, errors.New("no CPU or wall profile series ingested yet")
	}
	return data, nil
}

type selectSeriesResponse struct {
	Series []struct {
		Labels []labelPair `json:"labels"`
		Points []struct {
			Value     float64 `json:"value"`
			Timestamp string  `json:"timestamp"`
		} `json:"points"`
	} `json:"series"`
}

// selectSeries fetches a time series for the given service and profile type and
// returns the number of series and the total number of data points found.
func (e *env) selectSeries(ctx context.Context, host, service, profileType string) (nSeries, nPoints int, err error) {
	start, end := nowWindowMillis()
	var resp selectSeriesResponse
	err = e.pyroscopePost(ctx, host, "querier.v1.QuerierService/SelectSeries", map[string]any{
		"profileTypeID": profileType,
		"labelSelector": fmt.Sprintf("{service_name=%q}", service),
		"start":         start,
		"end":           end,
		"step":          60,
	}, &resp)
	if err != nil {
		return 0, 0, err
	}
	for _, s := range resp.Series {
		nPoints += len(s.Points)
	}
	return len(resp.Series), nPoints, nil
}

type flamegraphResponse struct {
	Flamegraph struct {
		Names  []string `json:"names"`
		Levels []struct {
			Values []json.Number `json:"values"`
		} `json:"levels"`
	} `json:"flamegraph"`
}

// selectMergeSpanProfile fetches a span-scoped profile for the given span IDs and
// returns the total number of samples at the root of the flame graph.
func (e *env) selectMergeSpanProfile(ctx context.Context, host, service, profileType string, spanIDs []string) (int64, error) {
	start, end := nowWindowMillis()
	var resp flamegraphResponse
	err := e.pyroscopePost(ctx, host, "querier.v1.QuerierService/SelectMergeSpanProfile", map[string]any{
		"profileTypeID": profileType,
		"labelSelector": fmt.Sprintf("{service_name=%q}", service),
		"spanSelector":  spanIDs,
		"start":         start,
		"end":           end,
	}, &resp)
	if err != nil {
		return 0, err
	}
	if len(resp.Flamegraph.Levels) == 0 || len(resp.Flamegraph.Levels[0].Values) < 2 {
		return 0, nil
	}
	// levels[0].values = [offset, total, self, nameIndex]; index 1 is the total.
	total, err := resp.Flamegraph.Levels[0].Values[1].Int64()
	if err != nil {
		return 0, err
	}
	return total, nil
}

// --- Tempo query helpers -----------------------------------------------------

type tempoSearchResponse struct {
	Traces []struct {
		TraceID string `json:"traceID"`
	} `json:"traces"`
}

func (e *env) tempoSearch(ctx context.Context, host string) ([]string, error) {
	start := time.Now().Add(-1 * time.Hour).Unix()
	end := time.Now().Add(1 * time.Minute).Unix()
	url := fmt.Sprintf("http://%s/api/search?start=%d&end=%d&limit=20", host, start, end)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tempo search returned %d: %s", resp.StatusCode, string(data))
	}
	var sr tempoSearchResponse
	if err := json.Unmarshal(data, &sr); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(sr.Traces))
	for _, tr := range sr.Traces {
		ids = append(ids, tr.TraceID)
	}
	return ids, nil
}

type otlpAttr struct {
	Key   string `json:"key"`
	Value struct {
		StringValue string `json:"stringValue"`
	} `json:"value"`
}

type tempoTrace struct {
	Batches []struct {
		ScopeSpans []struct {
			Spans []struct {
				Name       string     `json:"name"`
				Attributes []otlpAttr `json:"attributes"`
			} `json:"spans"`
		} `json:"scopeSpans"`
	} `json:"batches"`
}

const pyroscopeProfileIDAttr = "pyroscope.profile.id"

// profileIDsFromTrace fetches a trace and returns the values of every
// pyroscope.profile.id span attribute it contains.
func (e *env) profileIDsFromTrace(ctx context.Context, host, traceID string) ([]string, error) {
	url := fmt.Sprintf("http://%s/api/traces/%s", host, traceID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tempo trace returned %d: %s", resp.StatusCode, string(data))
	}
	var tr tempoTrace
	if err := json.Unmarshal(data, &tr); err != nil {
		return nil, err
	}
	var ids []string
	for _, b := range tr.Batches {
		for _, ss := range b.ScopeSpans {
			for _, sp := range ss.Spans {
				for _, a := range sp.Attributes {
					if a.Key == pyroscopeProfileIDAttr && a.Value.StringValue != "" {
						ids = append(ids, a.Value.StringValue)
					}
				}
			}
		}
	}
	return ids, nil
}

// --- Tests -------------------------------------------------------------------

// examplesToTest discovers the docker-compose examples and filters them by the
// PYROSCOPE_TEST_EXAMPLES environment variable (a comma/newline/space separated
// list of repository-relative paths).
func examplesToTest(t *testing.T) []*env {
	out, err := exec.Command("git", "ls-files", "**/docker-compose.yml", "**/docker-compose.yaml").Output()
	require.NoError(t, err)

	// requested maps each requested path to whether it matched an example.
	var requested map[string]bool
	if raw := strings.TrimSpace(os.Getenv("PYROSCOPE_TEST_EXAMPLES")); raw != "" {
		requested = map[string]bool{}
		for _, f := range strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == '\n' || r == ' ' || r == '\t'
		}) {
			requested[strings.TrimRight(f, "/")] = false
		}
	}

	var envs []*env
	for _, path := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if path == "" {
			continue
		}
		e := &env{dir: filepath.Dir(path), path: path}
		if requested != nil {
			matched := false
			for p := range requested {
				if e.repoDir() == p || strings.HasPrefix(e.repoDir(), p+"/") {
					requested[p] = true
					matched = true
				}
			}
			if !matched {
				continue
			}
		}
		envs = append(envs, e)
	}

	var unmatched []string
	for p, matched := range requested {
		if !matched {
			unmatched = append(unmatched, p)
		}
	}
	if len(unmatched) > 0 {
		sort.Strings(unmatched)
		t.Fatalf("PYROSCOPE_TEST_EXAMPLES references unknown example(s): %s", strings.Join(unmatched, ", "))
	}
	return envs
}

// isTracing reports whether the example lives under examples/tracing/.
func (e *env) isTracing() bool {
	return strings.HasPrefix(e.repoDir(), filepath.Join("examples", "tracing")+string(filepath.Separator))
}

// TestExamples brings each selected example up once and verifies it: profiles
// are always checked (Series + SelectSeries); tracing examples additionally get
// the trace-to-profile link checked (SelectMergeSpanProfile).
func TestExamples(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	for _, e := range examplesToTest(t) {
		e := e
		t.Run(e.repoDir(), func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), timeoutPerExample)
			defer cancel()

			e := e.prepareCompose(t)
			e.bringUp(t, ctx)
			e.waitRunning(t, ctx)

			pyroHost := "127.0.0.1:" + e.hostPort(t, ctx, pyroscopeService, pyroscopePort)
			t.Logf("[%s] all containers running; pyroscope on %s", e.repoDir(), pyroHost)

			t.Run("profiles", func(t *testing.T) {
				e.checkProfilesQueryable(t, ctx, pyroHost)
			})

			if e.isTracing() {
				tempoHost := "127.0.0.1:" + e.hostPort(t, ctx, tempoService, tempoPort)
				t.Run("trace-link", func(t *testing.T) {
					e.checkSpanProfilesQueryable(t, ctx, pyroHost, tempoHost)
				})
			}

			require.NoError(t, e.containersAllRunning(ctx), "containers must stay running through the run")
		})
	}
}

// checkProfilesQueryable verifies profiles are queryable from Pyroscope via the
// Series and SelectSeries APIs.
func (e *env) checkProfilesQueryable(t *testing.T, ctx context.Context, host string) {
	dir := e.repoDir()
	var summary string
	poll(t, ctx, profilesQueryTimeout, func(progress func(string, ...any)) error {
		progress("[%s] querying Series for ingested profiles...", dir)
		data, err := e.discoverSeries(ctx, host)
		if err != nil {
			return err
		}
		progress("[%s] Series found %d service(s): %s; querying SelectSeries...", dir, len(data), strings.Join(sortedKeys(data), ", "))
		for svc, types := range data {
			for _, pt := range types {
				nSeries, nPoints, err := e.selectSeries(ctx, host, svc, pt)
				if err != nil {
					return err
				}
				if nPoints > 0 {
					summary = fmt.Sprintf("service=%q profileType=%q -> %d series, %d points (%d service(s) ingesting)",
						svc, pt, nSeries, nPoints, len(data))
					return nil
				}
				progress("[%s] SelectSeries service=%q type=%q -> 0 points (waiting for data)", dir, svc, pt)
			}
		}
		return fmt.Errorf("no data points for any of %d discovered series yet", len(data))
	})
	t.Logf("[%s] PASS profiles queryable via Series+SelectSeries: %s", dir, summary)
}

// checkSpanProfilesQueryable verifies the trace-to-profile link end to end: a
// trace is found in Tempo, a span carries a pyroscope.profile.id attribute, and
// SelectMergeSpanProfile returns span-scoped profiling data for it.
func (e *env) checkSpanProfilesQueryable(t *testing.T, ctx context.Context, pyroHost, tempoHost string) {
	dir := e.repoDir()
	var summary string
	poll(t, ctx, tracesQueryTimeout, func(progress func(string, ...any)) error {
		progress("[%s] querying Series for ingested profiles...", dir)
		data, err := e.discoverSeries(ctx, pyroHost)
		if err != nil {
			return err
		}
		progress("[%s] Series found %d service(s): %s; searching Tempo for traces...", dir, len(data), strings.Join(sortedKeys(data), ", "))

		traceIDs, err := e.tempoSearch(ctx, tempoHost)
		if err != nil {
			return err
		}
		if len(traceIDs) == 0 {
			return errors.New("no traces found in tempo yet")
		}
		progress("[%s] found %d trace(s); scanning spans for %s attribute...", dir, len(traceIDs), pyroscopeProfileIDAttr)

		var spanIDs []string
		for _, id := range traceIDs {
			ids, err := e.profileIDsFromTrace(ctx, tempoHost, id)
			if err != nil {
				return err
			}
			spanIDs = append(spanIDs, ids...)
		}
		if len(spanIDs) == 0 {
			return fmt.Errorf("found %d traces in tempo but no span carried a %s attribute", len(traceIDs), pyroscopeProfileIDAttr)
		}
		progress("[%s] found %d span(s) with %s; querying SelectMergeSpanProfile...", dir, len(spanIDs), pyroscopeProfileIDAttr)

		for svc, types := range data {
			for _, pt := range types {
				total, err := e.selectMergeSpanProfile(ctx, pyroHost, svc, pt, spanIDs)
				if err != nil {
					return err
				}
				if total > 0 {
					summary = fmt.Sprintf("service=%q profileType=%q -> %d span id(s) from %d trace(s), %d samples",
						svc, pt, len(spanIDs), len(traceIDs), total)
					return nil
				}
				progress("[%s] SelectMergeSpanProfile service=%q type=%q -> 0 samples", dir, svc, pt)
			}
		}
		return fmt.Errorf("found %d span ids but SelectMergeSpanProfile returned no samples across %d service(s)", len(spanIDs), len(data))
	})
	t.Logf("[%s] PASS trace->profile link via SelectMergeSpanProfile: %s", dir, summary)
}
