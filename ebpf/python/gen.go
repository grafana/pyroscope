package python

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -type global_config_t -type libc -type py_str_type -type py_event -type py_offset_config -target amd64 -cc clang -cflags "-O2 -Wall -Werror -fpie -Wno-unused-variable -Wno-unused-function" Perf ../bpf/pyperf.bpf.c -- -I../bpf/libbpf -I../bpf/vmlinux/
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -type global_config_t -type libc -type py_str_type -type py_event -type py_offset_config -target arm64 -cc clang -cflags "-O2 -Wall -Werror -fpie -Wno-unused-variable -Wno-unused-function" Perf ../bpf/pyperf.bpf.c -- -I../bpf/libbpf -I../bpf/vmlinux/
