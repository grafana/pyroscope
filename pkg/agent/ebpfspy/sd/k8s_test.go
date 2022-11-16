package sd

import "testing"

func TestKubePodsCgroupsV1(t *testing.T) {
	cg := "11:devices:/kubepods/besteffort/pod85adbef3-622f-4ef2-8f60-a8bdf3eb6c72/" +
		"7edda1de1e0d1d366351e478359cf5fa16bb8ab53063a99bb119e56971bfb7e2"
	cid := getContainerIDFromCGroup(cg)
	expected := "7edda1de1e0d1d366351e478359cf5fa16bb8ab53063a99bb119e56971bfb7e2"
	if cid != expected {
		t.Fatalf("wrong cid %s != %s", cid, expected)
	}
}

func TestContainerdCgroupsV2(t *testing.T) {
	cg := "0::/kubepods.slice/kubepods-burstable.slice/" +
		"kubepods-burstable-podf9a04ecc_1875_491b_926c_d2f64757704e.slice/" +
		"cri-containerd-47e320f795efcec1ecf2001c3a09c95e3701ed87de8256837b70b10e23818251.scope"
	cid := getContainerIDFromCGroup(cg)
	expected := "47e320f795efcec1ecf2001c3a09c95e3701ed87de8256837b70b10e23818251"
	if cid != expected {
		t.Fatalf("wrong cid %s != %s", cid, expected)
	}
}

func TestDockerCgroupsV2(t *testing.T) {
	cg := "0::/../../kubepods-besteffort-pod88f6f4e3_59c0_4ce8_9ecf_391c8b5a60ad.slice/" +
		"docker-656959d9ee87a0b131c601ce9d9f8f76b1dda60e8608c503b5979d849cbdc714.scope"
	cid := getContainerIDFromCGroup(cg)
	expected := "656959d9ee87a0b131c601ce9d9f8f76b1dda60e8608c503b5979d849cbdc714"
	if cid != expected {
		t.Fatalf("wrong cid %s != %s", cid, expected)
	}
}

func TestCRI(t *testing.T) {
	statusContainerID := "containerd://a534eb629135e43beb13213976e37bb2ab95cba4c0d1d0b4e27c6bc4d8091b83"
	cgroup := "12:cpuset:/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod471203d1_984f_477e_9c35_db96487ffe5e.slice/" +
		"cri-containerd-a534eb629135e43beb13213976e37bb2ab95cba4c0d1d0b4e27c6bc4d8091b83.scope"
	cid := getContainerIDFromCGroup(cgroup)
	expected := "a534eb629135e43beb13213976e37bb2ab95cba4c0d1d0b4e27c6bc4d8091b83"
	if cid != expected {
		t.Fatalf("wrong cid %s != %s", cid, expected)
	}
	cid, err := getContainerIDFromK8S(statusContainerID)
	if err != nil {
		t.Fatal(err)
	}
	if cid != expected {
		t.Fatalf("wrong cid %s != %s", cid, expected)
	}
}
