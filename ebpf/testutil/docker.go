package testutil

import (
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"
)

type Container struct {
	T             *testing.T
	L             log.Logger
	ContainerID   string
	ContainerPort string
	hostPort      string
}

func RunContainerWithPort(t *testing.T, l log.Logger, image string, port string) *Container {
	container := &Container{
		T: t,
		L: log.With(l, "component", "docker"),
	}
	_, err := container.execute("docker", "pull", image)
	require.NoError(t, err)

	container.Run("docker", "run", "--rm", "-tid", "-p", port, image)

	container.ContainerPort = port
	container.WaitForPort()
	return container

}
func (c *Container) Run(cmd ...string) {
	out, err := c.execute(cmd...)
	require.NoError(c.T, err)
	c.ContainerID = strings.TrimSpace(string(out))
}

func (c *Container) Kill() {
	_, err := c.execute("docker", "kill", c.ContainerID)
	require.NoError(c.T, err)
}

func (c *Container) Url() string {
	return fmt.Sprintf("http://127.0.0.1:%s", c.HostPort())
}

func (c *Container) HostPort() string {
	if c.hostPort != "" {
		return c.hostPort
	}
	require.NotEqual(c.T, "", c.ContainerPort)
	out, err := c.execute("docker", "port", c.ContainerID, c.ContainerPort)
	require.NoError(c.T, err)
	ports := strings.Split(string(out), "\n")
	require.Greater(c.T, len(ports), 1)
	port := ports[0]
	fields := strings.Split(port, ":")
	require.Equal(c.T, len(fields), 2)
	c.hostPort = fields[1]
	return fields[1]
}

func (c *Container) WaitForPort() {
	require.Eventually(c.T, func() bool {
		req, err := http.NewRequest("GET", c.Url(), nil)
		require.NoError(c.T, err)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false
		}
		if resp.StatusCode != 200 {
			return false
		}
		_, err = io.ReadAll(resp.Body)
		require.NoError(c.T, err)
		return true
	}, 10*time.Second, 100*time.Millisecond)
}

func (c *Container) execute(cmd ...string) ([]byte, error) {
	cc := exec.Command(cmd[0], cmd[1:]...)
	out, err := cc.CombinedOutput()
	_ = c.L.Log("cmd", cc.String(), "output", string(out), "err", err)
	if err != nil {
		return nil, fmt.Errorf("%s run failed: %s - %w", cc.String(), string(out), err)
	}
	return out, nil
}
