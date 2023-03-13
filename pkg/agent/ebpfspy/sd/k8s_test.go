package sd

import (
	"fmt"
	"testing"
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
			cid := getContainerIDFromCGroup(tc.cgroup)
			expected := tc.expectedID
			if cid != expected {
				t.Errorf("wrong cid %s != %s", cid, expected)
			}
			cid, err := getContainerIDFromK8S(tc.containerID)
			if err != nil {
				t.Error(err)
			}
			if cid != expected {
				t.Errorf("wrong cid %s != %s", cid, expected)
			}
		})
	}
}
