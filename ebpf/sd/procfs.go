package sd

import (
	"bufio"
	"fmt"
	"regexp"
)

var (
	// cgroupContainerIDRe matches a container ID from a /proc/{pid}}/cgroup
	cgroupContainerIDRe = regexp.MustCompile(`^.*/(?:.*-)?([0-9a-f]{64})(?:\.|\s*$)`)
)

func (tf *targetFinder) getContainerIDFromPID(pid uint32) containerID {
	f, err := tf.fs.Open(fmt.Sprintf("proc/%d/cgroup", pid))
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		cid := getContainerIDFromCGroup(line)
		if cid != "" {
			return containerID(cid)
		}
	}
	return ""
}

func getContainerIDFromCGroup(line []byte) string {
	matches := cgroupContainerIDRe.FindSubmatch(line)
	if len(matches) <= 1 {
		return ""
	}
	return string(matches[1])
}
