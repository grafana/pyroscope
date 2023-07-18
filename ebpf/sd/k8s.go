package sd

import "strings"

var knownContainerIDPrefixes = []string{"docker://", "containerd://", "cri-o://"}

// get container id from __meta_kubernetes_pod_container_id label
func getContainerIDFromK8S(k8sContainerID string) containerID {
	for _, p := range knownContainerIDPrefixes {
		if strings.HasPrefix(k8sContainerID, p) {
			return containerID(strings.TrimPrefix(k8sContainerID, p))
		}
	}
	return ""
}
