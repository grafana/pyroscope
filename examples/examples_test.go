//go:build examples

package examples

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"os/exec"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const (
	timeoutPerExample     = 10 * time.Minute
	durationToStayRunning = 5 * time.Second
)

type env struct {
	dir  string // project dir of docker-compose
	path string // path to docker-compose file
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

func (e *env) newCmdWithOutputCapture(t testing.TB, ctx context.Context, args ...string) *exec.Cmd {
	c := e.newCmd(ctx, args...)
	stdout, err := c.StdoutPipe()
	require.NoError(t, err)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			t.Log(scanner.Text())
		}
	}()

	stderr, err := c.StderrPipe()
	require.NoError(t, err)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			t.Log("STDERR: " + scanner.Text())
		}
	}()

	return c
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

	var errs []error
	for _, s := range status {
		if s.State != "running" {
			errs = append(errs, fmt.Errorf("container %s is not running", s.Name))
		}
	}

	return errors.Join(errs...)
}

// removeExposedPorts removes ports from services which expose fixed ports. This will break once there is an overlap of ports. This will instead use random ports allocated by docker-compose.
func (e *env) removeExposedPorts(t testing.TB) *env {

	var obj map[interface{}]interface{}

	body, err := os.ReadFile(e.path)
	if err != nil {
		require.NoError(t, err)
	}

	if err := yaml.Unmarshal(body, &obj); err != nil {
		require.NoError(t, err)
	}

	changed := false

	for key, value := range obj {
		if key.(string) == "services" {
			services, ok := value.(map[string]interface{})
			if !ok {
				require.NoError(t, fmt.Errorf("services is not a map[string]interface{}"))
			}
			for serviceName, service := range services {
				params, ok := service.(map[string]interface{})
				if !ok {
					require.NoError(t, fmt.Errorf("service '%s' is not a map[string]interface{}", serviceName))
				}

				// check for ports
				ports, ok := params["ports"]
				if !ok {
					continue
				}

				portsSlice, ok := ports.([]interface{})
				if !ok {
					continue
				}
				for i := range portsSlice {
					port, ok := portsSlice[i].(string)
					if !ok {
						continue
					}

					portSplitted := strings.Split(port, ":")
					if len(portSplitted) < 2 {
						continue
					}

					portsSlice[i] = portSplitted[len(portSplitted)-1]
					changed = true
				}
			}
		}

	}
	if !changed {
		return e
	}

	path := filepath.Join(t.TempDir(), "docker-compose.yml")
	data, err := yaml.Marshal(obj)
	if err != nil {
		require.NoError(t, err)
	}

	require.NoError(t, os.WriteFile(path, data, 0644))

	return &env{
		dir:  e.dir,
		path: path,
	}
}

// This test is meant to catch very fundamental errors in the examples. It could be extened to be more comprehensive. For now it will just run the examples and check that they don't crash, within 5 seconds.
func TestDockerComposeBuildRun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	ctx := context.Background()

	// find docker compose files
	out, err := exec.Command("git", "ls-files", "**/docker-compose.yml").Output()
	require.NoError(t, err)

	var envs []*env
	for _, path := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		e := &env{dir: filepath.Dir(path), path: path}
		envs = append(envs, e)
	}

	for i := range envs {
		t.Run(envs[i].dir, func(t *testing.T) {
			e := envs[i]
			t.Parallel()
			ctx, cancel := context.WithTimeout(ctx, timeoutPerExample)
			defer cancel()
			t.Run("build", func(t *testing.T) {
				cmd := e.newCmdWithOutputCapture(t, ctx, "build")
				require.NoError(t, cmd.Run())
			})
			// run pull first so lcontainers can start immediately
			t.Run("pull", func(t *testing.T) {
				cmd := e.newCmdWithOutputCapture(t, ctx, "pull")
				require.NoError(t, cmd.Run())
			})
			// now run the docker-compose containers, run them for 5 seconds, it would abort if one of the containers exits
			t.Run("run", func(t *testing.T) {
				ctx, cancel := context.WithCancel(ctx)
				defer cancel()
				e = e.removeExposedPorts(t)
				cmd := e.newCmdWithOutputCapture(t, ctx, "up", "--abort-on-container-exit")
				require.NoError(t, cmd.Start())

				// cleanup what ever happens
				defer func() {
					err := e.newCmdWithOutputCapture(t, context.Background(), "down", "--volumes").Run()
					if err != nil {
						t.Logf("cleanup error=%v\n", err)
					}
				}()

				// check if all containers are still running after 5 seconds
				go func() {
					<-time.After(durationToStayRunning)
					err := e.containersAllRunning(ctx)
					if err != nil {
						t.Logf("do nothing, as not all containers are running: %v\n", err)
						return
					}
					t.Log("all healthy, start graceful shutdown")
					err = cmd.Process.Signal(syscall.SIGTERM)
					if err != nil {
						t.Log("error sending terminate signal", err)
					}
				}()

				err := cmd.Wait()
				var exitError *exec.ExitError
				if !errors.As(err, &exitError) || exitError.ExitCode() != 130 {
					require.NoError(t, err)
				}

			})
		})
	}

}
