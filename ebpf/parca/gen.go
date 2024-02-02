package parca


//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -type unwinder_config_t -target amd64 -cc clang -cflags "-O2 -Wall -Werror -fpie -Wno-unused-variable -Wno-unused-function" ParcaNative ../../parca-agent/bpf/unwinders/native.bpf.c -- -I../../parca-agent/bpf/unwinders -I../bpf/libbpf -I../bpf/vmlinux/ -I../bpf/parca
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -type unwinder_config_t -target arm64 -cc clang -cflags "-O2 -Wall -Werror -fpie -Wno-unused-variable -Wno-unused-function" ParcaNative ../../parca-agent/bpf/unwinders/native.bpf.c -- -I../../parca-agent/bpf/unwinders -I../bpf/libbpf -I../bpf/vmlinux/ -I../bpf/parca

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target amd64 -cc clang -cflags "-O2 -Wall -Werror -fpie -Wno-unused-variable -Wno-unused-function" ParcaRuby ../../parca-agent/bpf/unwinders/rbperf.bpf.c -- -I../../parca-agent/bpf/unwinders -I../bpf/libbpf -I../bpf/vmlinux/ -I../bpf/parca
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target arm64 -cc clang -cflags "-O2 -Wall -Werror -fpie -Wno-unused-variable -Wno-unused-function" ParcaRuby ../../parca-agent/bpf/unwinders/rbperf.bpf.c -- -I../../parca-agent/bpf/unwinders -I../bpf/libbpf -I../bpf/vmlinux/ -I../bpf/parca

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target amd64 -cc clang -cflags "-O2 -Wall -Werror -fpie -Wno-unused-variable -Wno-unused-function" ParcaPython ../../parca-agent/bpf/unwinders/pyperf.bpf.c -- -I../../parca-agent/bpf/unwinders -I../bpf/libbpf -I../bpf/vmlinux/ -I../bpf/parca
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target arm64 -cc clang -cflags "-O2 -Wall -Werror -fpie -Wno-unused-variable -Wno-unused-function" ParcaPython ../../parca-agent/bpf/unwinders/pyperf.bpf.c -- -I../../parca-agent/bpf/unwinders -I../bpf/libbpf -I../bpf/vmlinux/ -I../bpf/parca
