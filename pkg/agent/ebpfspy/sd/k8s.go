package sd

import (
	"bufio"
	"context"
	"fmt"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"os"
	"regexp"
	"strings"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type K8SServiceDiscovery struct {
	cs                 *kubernetes.Clientset
	nodeName           string
	containerID2Labels map[string]*spy.Labels
	pid2Labels         map[uint32]*spy.Labels
}

var knownContainerIDPrefixes = []string{"docker://", "containerd://"}
var knownRuntimes = []string{"docker://", "containerd://"}

func NewK8ServiceDiscovery(ctx context.Context, nodeName string) (ServiceDiscovery, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	node, err := clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	criVersion := node.Status.NodeInfo.ContainerRuntimeVersion

	if !isKnownContainerRuntime(criVersion) {
		return nil, fmt.Errorf("unknown cri %s", criVersion)
	}

	return &K8SServiceDiscovery{
		cs:                 clientset,
		nodeName:           nodeName,
		containerID2Labels: map[string]*spy.Labels{},
		pid2Labels:         map[uint32]*spy.Labels{},
	}, nil
}

func (sd *K8SServiceDiscovery) Refresh(ctx context.Context) error {
	//todo make it async - it is io bound?
	sd.containerID2Labels = map[string]*spy.Labels{}
	sd.pid2Labels = map[uint32]*spy.Labels{}
	pods, err := sd.cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.nodeName", sd.nodeName).String(),
	})
	if err != nil {
		return err
	}

	for _, pod := range pods.Items {
		for _, status := range pod.Status.ContainerStatuses {
			cid, err := getContainerIDFromK8S(status.ContainerID)
			if err != nil {
				return err
			}
			ls := spy.NewLabels()
			ls.Set("k8s_node", sd.nodeName)
			ls.Set("k8s_pod_name", pod.Name)
			ls.Set("k8s_pod_namespace", pod.Namespace)
			ls.Set("k8s_container_id", cid)
			ls.Set("k8s_container_name", status.Name)
			if v, ok := pod.Labels["app.kubernetes.io/name"]; ok {
				ls.Set("k8s_app_name", v)
			}
			if v, ok := pod.Labels["app.kubernetes.io/version"]; ok {
				ls.Set("k8s_app_version", v)
			}
			sd.containerID2Labels[cid] = ls
		}
	}
	return nil
}

func (sd *K8SServiceDiscovery) GetLabels(pid uint32) *spy.Labels {
	ls, ok := sd.pid2Labels[pid]
	if ok {
		return ls
	}
	cid := getContainerIDFromPID(pid)

	if cid == "" {
		sd.pid2Labels[pid] = nil
		return nil
	}
	ls, ok = sd.containerID2Labels[cid]
	sd.pid2Labels[pid] = ls
	return ls
}

func isKnownContainerRuntime(criVersion string) bool {
	for _, runtime := range knownRuntimes {
		if strings.HasPrefix(criVersion, runtime) {
			return true
		}
	}
	return false
}

func getContainerIDFromK8S(k8sContainerID string) (string, error) {
	for _, p := range knownContainerIDPrefixes {
		if strings.HasPrefix(k8sContainerID, p) {
			return strings.TrimPrefix(k8sContainerID, p), nil
		}
	}
	return "", fmt.Errorf("unknown container id %s", k8sContainerID)
}

func getContainerIDFromPID(pid uint32) string {
	f, err := os.Open(fmt.Sprintf("/proc/%d/cgroup", pid))
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		cid := getContainerIDFromCGroup(line)
		if cid != "" {
			return cid
		}
	}
	return ""
}

func getContainerIDFromCGroup(line string) string {
	parts := dockerPattern.FindStringSubmatch(line)
	if parts != nil {
		return parts[1]
	}
	parts = kubePattern.FindStringSubmatch(line)
	if parts != nil {
		return parts[1]
	}
	parts = cgroupV2ScopePattern.FindStringSubmatch(line)
	if parts != nil {
		return parts[1]
	}
	return ""
}

var (
	kubePattern          = regexp.MustCompile(`\d+:.+:/kubepods/[^/]+/pod[^/]+/([0-9a-f]{64})`)
	dockerPattern        = regexp.MustCompile(`\d+:.+:/docker/pod[^/]+/([0-9a-f]{64})`)
	cgroupV2ScopePattern = regexp.MustCompile(`^0::.*/(?:docker-|cri-containerd-)([0-9a-f]{64})\.scope$`)
)
