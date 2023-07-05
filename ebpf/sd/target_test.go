package sd

import (
	"fmt"
	"github.com/grafana/phlare/ebpf/util"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCGroupMatching(t *testing.T) {
	type testcase = struct {
		containerID, cgroup, expectedID string
	}
	testcases := []testcase{
		{
			containerID: "containerd://a534eb629135e43beb13213976e37bb2ab95cba4c0d1d0b4e27c6bc4d8091b83",
			cgroup: "12:cpuset:/kubepods.slice/kubepods-burstable.slice/" +
				"kubepods-burstable-pod471203d1_984f_477e_9c35_db96487ffe5e.slice/" +
				"cri-containerd-a534eb629135e43beb13213976e37bb2ab95cba4c0d1d0b4e27c6bc4d8091b83.scope",
			expectedID: "a534eb629135e43beb13213976e37bb2ab95cba4c0d1d0b4e27c6bc4d8091b83",
		},
		{
			containerID: "cri-o://0ecc7949cbaf17e883264ea1055f60b184a7cb264fd759c4a692e1155086fe2d",
			cgroup: "0::/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-podb57320a0_e7eb_4ac8_a791_4c4472796867.slice/" +
				"crio-0ecc7949cbaf17e883264ea1055f60b184a7cb264fd759c4a692e1155086fe2d.scope",
			expectedID: "0ecc7949cbaf17e883264ea1055f60b184a7cb264fd759c4a692e1155086fe2d",
		},
		{

			containerID: "docker://656959d9ee87a0b131c601ce9d9f8f76b1dda60e8608c503b5979d849cbdc714",
			cgroup: "0::/../../kubepods-besteffort-pod88f6f4e3_59c0_4ce8_9ecf_391c8b5a60ad.slice/" +
				"docker-656959d9ee87a0b131c601ce9d9f8f76b1dda60e8608c503b5979d849cbdc714.scope",
			expectedID: "656959d9ee87a0b131c601ce9d9f8f76b1dda60e8608c503b5979d849cbdc714",
		},
		{
			containerID: "containerd://47e320f795efcec1ecf2001c3a09c95e3701ed87de8256837b70b10e23818251",
			cgroup: "0::/kubepods.slice/kubepods-burstable.slice/" +
				"kubepods-burstable-podf9a04ecc_1875_491b_926c_d2f64757704e.slice/" +
				"cri-containerd-47e320f795efcec1ecf2001c3a09c95e3701ed87de8256837b70b10e23818251.scope",
			expectedID: "47e320f795efcec1ecf2001c3a09c95e3701ed87de8256837b70b10e23818251",
		},
		{
			containerID: "docker://7edda1de1e0d1d366351e478359cf5fa16bb8ab53063a99bb119e56971bfb7e2",
			cgroup: "11:devices:/kubepods/besteffort/pod85adbef3-622f-4ef2-8f60-a8bdf3eb6c72/" +
				"7edda1de1e0d1d366351e478359cf5fa16bb8ab53063a99bb119e56971bfb7e2",
			expectedID: "7edda1de1e0d1d366351e478359cf5fa16bb8ab53063a99bb119e56971bfb7e2",
		},
	}
	for i, tc := range testcases {
		t.Run(fmt.Sprintf("testcase %d %s", i, tc.cgroup), func(t *testing.T) {
			cid := getContainerIDFromCGroup([]byte(tc.cgroup))
			expected := tc.expectedID
			require.Equal(t, expected, cid)
			cid = string(getContainerIDFromK8S(tc.containerID))
			require.Equal(t, expected, cid)
		})
	}
}

type mockFS struct {
	root     fs.FS
	rootPath string
}

func newMockFS() (*mockFS, error) {
	temp, err := os.MkdirTemp("", "TestTargetFinder")
	if err != nil {
		return nil, err
	}
	return &mockFS{
		rootPath: temp,
		root:     os.DirFS(temp),
	}, nil
}

func (fs *mockFS) add(path string, data []byte) error {
	fpath := filepath.Join(fs.rootPath, path)
	dir := filepath.Dir(fpath)
	if _, err := os.Stat(dir); err != nil {
		err = os.MkdirAll(dir, 0770)
		if err != nil {
			return err
		}
	}
	return os.WriteFile(fpath, data, 0660)
}

func (fs *mockFS) rm() {
	_ = os.RemoveAll(fs.rootPath)
}

func TestTargetFinder(t *testing.T) {
	fs, err := newMockFS()
	require.NoError(t, err)
	defer fs.rm()
	err = fs.add("/proc/1801264/cgroup",
		[]byte("12:blkio:/kubepods/burstable/pod7e5f5ac0-1af4-49ab-8938-664970a26cfd/9a7c72f122922fe3445ba85ce72c507c8976c0f3d919403fda7c22dfe516f66f"))
	require.NoError(t, err)
	err = fs.add("/proc/489323/cgroup",
		[]byte("12:blkio:/kubepods/burstable/pod83ca8044-3e7c-457b-8647-a21dabad5079/57ac76ffc93d7e7735ca186bc67115656967fc8aecbe1f65526c4c48b033e6a5"))
	require.NoError(t, err)

	options := TargetsOptions{
		Targets: []DiscoveryTarget{
			map[string]string{
				"__meta_kubernetes_pod_container_id":   "containerd://9a7c72f122922fe3445ba85ce72c507c8976c0f3d919403fda7c22dfe516f66f",
				"__meta_kubernetes_namespace":          "foo",
				"__meta_kubernetes_pod_container_name": "bar",
			},
			map[string]string{
				"__container_id__":                     "57ac76ffc93d7e7735ca186bc67115656967fc8aecbe1f65526c4c48b033e6a5",
				"__meta_kubernetes_namespace":          "qwe",
				"__meta_kubernetes_pod_container_name": "asd",
			},
		},
		TargetsOnly:        true,
		DefaultTarget:      nil,
		ContainerCacheSize: 1024,
	}

	tf, err := NewTargetFinder(fs.root, util.TestLogger(t), options)
	require.NoError(t, err)

	target := tf.FindTarget(1801264)
	require.NotNil(t, target)
	require.Equal(t, "ebpf/foo/bar", target.labels.Get("service_name"))

	target = tf.FindTarget(489323)
	require.NotNil(t, target)
	require.Equal(t, "ebpf/qwe/asd", target.labels.Get("service_name"))

	target = tf.FindTarget(239)
	require.Nil(t, target)
}
