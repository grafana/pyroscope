package sd

import (
	"bufio"
	"fmt"
	"regexp"
)

var (
	// cgroupContainerIDRe matches a container ID from a /proc/{pid}}/cgroup
	// 12:cpuset:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod471203d1_984f_477e_9c35_db96487ffe5e.slice/cri-containerd-a534eb629135e43beb13213976e37bb2ab95cba4c0d1d0b4e27c6bc4d8091b83.scope"
	// 11:devices:/kubepods/besteffort/pod85adbef3-622f-4ef2-8f60-a8bdf3eb6c72/7edda1de1e0d1d366351e478359cf5fa16bb8ab53063a99bb119e56971bfb7e2
	cgroupContainerIDRe = regexp.MustCompile(`^.*/(?:.*-)?([0-9a-f]+)(?:\.|\s*$)`)
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
