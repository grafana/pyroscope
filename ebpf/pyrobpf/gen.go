package pyrobpf

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go   -target amd64 -cc clang-17 -cflags "-O2 -Wall -Werror -fpie -Wno-unused-variable -Wno-unused-function" Profile ../bpf/profile.bpf.c -- -I../bpf/libbpf -I../bpf/vmlinux/
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go   -target arm64 -cc clang-17 -cflags "-O2 -Wall -Werror -fpie -Wno-unused-variable -Wno-unused-function" Profile ../bpf/profile.bpf.c -- -I../bpf/libbpf -I../bpf/vmlinux/
