package sd

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
)

type DiscoveryTarget map[string]string

// DebugString return unsorted labels as a string.
func (t *DiscoveryTarget) DebugString() string {
	var b strings.Builder
	b.WriteByte('{')
	for k, v := range *t {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(v)
		b.WriteByte(',')
	}
	b.WriteByte('}')
	return b.String()
}

const (
	labelContainerID    = "__container_id__"
	labelServiceName    = "service_name"
	labelServiceNameK8s = "__meta_kubernetes_pod_annotation_pyroscope_io_service_name"
	metricValue         = "process_cpu"
)

type Target struct {

	// todo make keep it a map until Append happens
	labels                labels.Labels
	serviceName           string
	fingerprint           uint64
	fingerprintCalculated bool
}

func NewTarget(cid containerID, target DiscoveryTarget) (*Target, error) {
	serviceName := target[labelServiceName]
	if serviceName == "" {
		serviceName = inferServiceName(target)
	}

	lset := make(map[string]string, len(target))
	for k, v := range target {
		if strings.HasPrefix(k, model.ReservedLabelPrefix) && k != labels.MetricName {
			continue
		}
		lset[k] = v
	}
	if lset[labels.MetricName] == "" {
		lset[labels.MetricName] = metricValue
	}
	if lset[labelServiceName] == "" {
		lset[labelServiceName] = serviceName
	}
	if cid != "" {
		lset[labelContainerID] = string(cid)
	}
	return &Target{
		labels:      labels.FromMap(lset),
		serviceName: serviceName,
	}, nil
}

func (t *Target) ServiceName() string {
	return t.serviceName
}

func inferServiceName(target DiscoveryTarget) string {
	k8sServiceName := target[labelServiceNameK8s]
	if k8sServiceName != "" {
		return k8sServiceName
	}
	k8sNamespace := target["__meta_kubernetes_namespace"]
	k8sContainer := target["__meta_kubernetes_pod_container_name"]
	if k8sNamespace != "" && k8sContainer != "" {
		return fmt.Sprintf("ebpf/%s/%s", k8sNamespace, k8sContainer)
	}
	dockerContainer := target["__meta_docker_container_name"]
	if dockerContainer != "" {
		return dockerContainer
	}
	return "unspecified"
}

func (t *Target) Labels() (uint64, labels.Labels) {
	if !t.fingerprintCalculated {
		t.fingerprint = t.labels.Hash()
		t.fingerprintCalculated = true
	}
	return t.fingerprint, t.labels
}

type containerID string

type TargetFinder interface {
	FindTarget(pid uint32) *Target
	DebugInfo() []string
	Update(args TargetsOptions)
}
type TargetsOptions struct {
	Targets            []DiscoveryTarget
	TargetsOnly        bool
	DefaultTarget      DiscoveryTarget
	ContainerCacheSize int
}

type targetFinder struct {
	l          log.Logger
	cid2target map[containerID]*Target

	// todo make it never evict during a reset
	containerIDCache *lru.Cache[uint32, containerID]
	defaultTarget    *Target
	fs               fs.FS
}

func (tf *targetFinder) Update(args TargetsOptions) {
	tf.setTargets(args)
	tf.resizeContainerIDCache(args.ContainerCacheSize)
}

func NewTargetFinder(fs fs.FS, l log.Logger, options TargetsOptions) (TargetFinder, error) {
	containerIDCache, err := lru.New[uint32, containerID](options.ContainerCacheSize)
	if err != nil {
		return nil, fmt.Errorf("containerIDCache create: %w", err)
	}
	res := &targetFinder{
		l:                l,
		containerIDCache: containerIDCache,
		fs:               fs,
	}
	res.setTargets(options)
	return res, nil
}

func (tf *targetFinder) setTargets(opts TargetsOptions) {
	_ = level.Debug(tf.l).Log("msg", "set targets", "count", len(opts.Targets))
	containerID2Target := make(map[containerID]*Target)
	for _, target := range opts.Targets {
		cid := containerIDFromTarget(target)
		if cid != "" {
			t, err := NewTarget(cid, target)
			if err != nil {
				_ = level.Error(tf.l).Log(
					"msg", "target skipped",
					"target", target.DebugString(),
					"err", err,
				)
				continue
			}
			containerID2Target[cid] = t
		}
	}
	if len(opts.Targets) > 0 && len(containerID2Target) == 0 {
		_ = level.Warn(tf.l).Log("msg", "No container IDs found in targets")
	}
	tf.cid2target = containerID2Target
	if opts.TargetsOnly {
		tf.defaultTarget = nil
	} else {
		t, err := NewTarget("", opts.DefaultTarget)
		if err != nil {
			_ = level.Error(tf.l).Log(
				"msg", "default target skipped",
				"target", opts.DefaultTarget.DebugString(),
				"err", err,
			)
			tf.defaultTarget = nil
		} else {
			tf.defaultTarget = t
		}
	}
	_ = level.Debug(tf.l).Log("msg", "created targets", "count", len(tf.cid2target))
}

func (tf *targetFinder) FindTarget(pid uint32) *Target {
	res := tf.findTarget(pid)
	if res != nil {
		return res
	}
	return tf.defaultTarget
}

func (tf *targetFinder) findTarget(pid uint32) *Target {
	cid, ok := tf.containerIDCache.Get(pid)
	if ok {
		return tf.cid2target[cid]
	}

	cid = tf.getContainerIDFromPID(pid)
	tf.containerIDCache.Add(pid, cid)
	return tf.cid2target[cid]
}

func (tf *targetFinder) resizeContainerIDCache(size int) {
	tf.containerIDCache.Resize(size)
}

func (tf *targetFinder) DebugInfo() []string {
	debugTargets := make([]string, 0, len(tf.cid2target))
	for _, target := range tf.cid2target {
		_, ls := target.Labels()
		debugTargets = append(debugTargets, ls.String())
	}
	return debugTargets
}

func (tf *targetFinder) Targets() []*Target {
	res := make([]*Target, 0, len(tf.cid2target))
	for _, target := range tf.cid2target {
		res = append(res, target)
	}
	return res
}

func containerIDFromTarget(target DiscoveryTarget) containerID {
	cid, ok := target[labelContainerID]
	if ok && cid != "" {
		return containerID(cid)
	}
	cid, ok = target["__meta_kubernetes_pod_container_id"]
	if ok && cid != "" {
		return getContainerIDFromK8S(cid)
	}
	cid, ok = target["__meta_docker_container_id"]
	if ok && cid != "" {
		return containerID(cid)
	}
	return ""
}
