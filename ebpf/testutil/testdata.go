package testutil

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func InitGitSubmodule(t *testing.T) {
	cmd := exec.Command("git", "submodule", "update", "--init", "--recursive")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Run())
}
