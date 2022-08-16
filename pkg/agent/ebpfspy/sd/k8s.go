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

	//"k8s.io/apimachinery/pkg/fields"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type K8SServiceDiscovery struct {
	cs                 *kubernetes.Clientset
	nodeName           string
	containerID2Labels map[string]*spy.Labels
	pid2Labels         map[uint32]*spy.Labels
}

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
	fmt.Println("node.Status.NodeInfo.ContainerRuntimeVersion", criVersion)
	if !strings.HasPrefix(criVersion, "docker://") {
		return nil, fmt.Errorf("unknown cri %s", criVersion)
	}
	//fmt.Printf("%v\n", node)

	return &K8SServiceDiscovery{
		cs:                 clientset,
		nodeName:           nodeName,
		containerID2Labels: map[string]*spy.Labels{},
		pid2Labels:         map[uint32]*spy.Labels{},
	}, nil
}

func (sd *K8SServiceDiscovery) Refresh(ctx context.Context) error {
	const dockerCIDPrefix = "docker://"
	//todo make it async - it is io bound?
	//fmt.Printf("sd Refresh prev pidmap size: %d\n", len(sd.pid2Labels))

	sd.containerID2Labels = map[string]*spy.Labels{}
	sd.pid2Labels = map[uint32]*spy.Labels{}
	pods, err := sd.cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.nodeName", sd.nodeName).String(),
	})
	if err != nil {
		return err
	}

	for _, pod := range pods.Items {
		//fmt.Printf("ns: %s pod.Name %s\n", pod.Namespace, pod.Name)
		//fmt.Printf("full pod %+v\n", pod)
		for _, status := range pod.Status.ContainerStatuses {
			cid := status.ContainerID
			if !strings.HasPrefix(cid, dockerCIDPrefix) {
				return fmt.Errorf("unknown container id %s", cid)
			}
			cid = strings.TrimPrefix(cid, dockerCIDPrefix)
			//fmt.Printf("     container %s %s\n", cid, status.Name)
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
	//fmt.Printf("sd Refresh done %v\n", sd.containerID2Labels)
	return nil
}

func (sd *K8SServiceDiscovery) GetLabels(pid uint32) *spy.Labels {
	ls, ok := sd.pid2Labels[pid]
	if ok {
		return ls
	}
	cid := LookupDockerContainerID(pid)

	if cid == "" {
		//fmt.Printf("k8s sd pid %d resolved to nill\n", pid)
		sd.pid2Labels[pid] = nil
		return nil
	}
	ls, ok = sd.containerID2Labels[cid]
	sd.pid2Labels[pid] = ls
	//fmt.Printf("k8s sd pid %d resolved to %v\n%v\n", pid, ls, sd.containerID2Labels)
	return ls
}

func LookupDockerContainerID(pid uint32) string {
	f, err := os.Open(fmt.Sprintf("/proc/%d/cgroup", pid))
	if err != nil {
		//fmt.Printf("LookupDockerContainerID %v\n", err)

		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	res := ""
	for scanner.Scan() {
		line := scanner.Text()
		parts := dockerPattern.FindStringSubmatch(line)
		if parts != nil {
			res = parts[1]
		}
		parts = kubePattern.FindStringSubmatch(line)
		if parts != nil {
			res = parts[1]
		}
		parts = cgroupV2DockerScope.FindStringSubmatch(line)
		if parts != nil {
			res = parts[1]
		}
	}
	//fmt.Printf("LookupDockerContainerID %d %s\n", pid, res)

	return res
}

var (
	kubePattern         = regexp.MustCompile(`\d+:.+:/kubepods/[^/]+/pod[^/]+/([0-9a-f]{64})`)
	dockerPattern       = regexp.MustCompile(`\d+:.+:/docker/pod[^/]+/([0-9a-f]{64})`)
	cgroupV2DockerScope = regexp.MustCompile(`^0::.*/docker-([0-9a-f]{64})\.scope$`)
)
