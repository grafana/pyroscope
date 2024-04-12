package pyrobpf

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -type global_config_t -type pid_event -target amd64 -cc clang -cflags "-O2 -Wall -Werror -fpie -Wno-unused-variable -Wno-unused-function" Profile ../bpf/profile.bpf.c -- -I../bpf/libbpf -I../bpf/vmlinux/
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -type global_config_t -type pid_event -target arm64 -cc clang -cflags "-O2 -Wall -Werror -fpie -Wno-unused-variable -Wno-unused-function" Profile ../bpf/profile.bpf.c -- -I../bpf/libbpf -I../bpf/vmlinux/
