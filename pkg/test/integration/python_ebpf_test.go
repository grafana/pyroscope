package integration

import (
	"bytes"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

var images = []string{
	"korniltsev/ebpf-testdata-rideshare:3.8-slim",
	//"korniltsev/ebpf-testdata-rideshare:3.9-slim",
	//"korniltsev/ebpf-testdata-rideshare:3.10-slim",
	//"korniltsev/ebpf-testdata-rideshare:3.11-slim",
	//"korniltsev/ebpf-testdata-rideshare:3.12-slim",
	//"korniltsev/ebpf-testdata-rideshare:3.13-rc-slim",
	//"korniltsev/ebpf-testdata-rideshare:3.8-alpine",
	//"korniltsev/ebpf-testdata-rideshare:3.9-alpine",
	//"korniltsev/ebpf-testdata-rideshare:3.10-alpine",
	//"korniltsev/ebpf-testdata-rideshare:3.11-alpine",
	//"korniltsev/ebpf-testdata-rideshare:3.12-alpine",
	//"korniltsev/ebpf-testdata-rideshare:3.13-rc-alpine",
}

func TestPython(t *testing.T) {

}

func startContainer(t *testing.T, image string) string {
	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	cmd := exec.Command("docker", "run", "-d", "--rm", image)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	require.NoError(t, err, "docker run failed: %s %s", stdout.String(), stderr.String())
	return stdout.String()
}
