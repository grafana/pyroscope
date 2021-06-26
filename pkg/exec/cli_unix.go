// +build !windows

package exec

import (
	"os"
	goexec "os/exec"
	"os/user"
	"regexp"
	"strconv"
	"syscall"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func adjustCmd(cmd *goexec.Cmd, cfg config.Exec) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	// permissions drop
	if isRoot() && !cfg.NoRootDrop && os.Getenv("SUDO_UID") != "" && os.Getenv("SUDO_GID") != "" {
		creds, err := generateCredentialsDrop()
		if err != nil {
			logrus.Errorf("failed to drop permissions, %q", err)
		} else {
			cmd.SysProcAttr.Credential = creds
		}
	}

	if cfg.UserName != "" || cfg.GroupName != "" {
		creds, err := generateCredentials(cfg.UserName, cfg.GroupName)
		if err != nil {
			logrus.Errorf("failed to generate credentials: %q", err)
		} else {
			cmd.SysProcAttr.Credential = creds
		}
	}
	cmd.SysProcAttr.Setpgid = true
	return nil
}

func isRoot() bool {
	u, err := user.Current()
	return err == nil && u.Username == "root"
}

func generateCredentialsDrop() (*syscall.Credential, error) {
	sudoUser := os.Getenv("SUDO_USER")
	sudoUID := os.Getenv("SUDO_UID")
	sudoGid := os.Getenv("SUDO_GID")

	logrus.Infof("dropping permissions, running command as %q (%s/%s)", sudoUser, sudoUID, sudoGid)

	uid, err := strconv.Atoi(sudoUID)
	if err != nil {
		return nil, err
	}
	gid, err := strconv.Atoi(sudoGid)
	if err != nil {
		return nil, err
	}

	return &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}, nil
}

var digitCheck = regexp.MustCompile(`^[0-9]+$`)

func generateCredentials(userName, groupName string) (*syscall.Credential, error) {
	c := syscall.Credential{}

	var u *user.User
	var g *user.Group
	var err error

	if userName != "" {
		if digitCheck.MatchString(userName) {
			u, err = user.LookupId(userName)
		} else {
			u, err = user.Lookup(userName)
		}
		if err != nil {
			return nil, err
		}

		uid, _ := strconv.Atoi(u.Uid)
		c.Uid = uint32(uid)
	}

	if groupName != "" {
		if digitCheck.MatchString(groupName) {
			g, err = user.LookupGroupId(groupName)
		} else {
			g, err = user.LookupGroup(groupName)
		}
		if err != nil {
			return nil, err
		}

		gid, _ := strconv.Atoi(g.Gid)
		c.Gid = uint32(gid)
	}

	return &c, nil
}

func processExists(pid int) bool {
	return nil == syscall.Kill(pid, 0)
}
