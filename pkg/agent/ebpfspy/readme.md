
```
# how to build and run locally with shared libararies
# build bcc-syms, libbpf without installing globally
make -C bpf libs/bcc-syms libs/libbpf
# build the ebpf binary
make -C bpf profile.bpf.o
# build pyroscope with ebpfspy support
ENABLED_SPIES=ebpfspy  make build
# if bcc-syms and libbpf are not installed globally - point linker to them
export LD_LIBRARY_PATH=$(pyroscope_dir)/pkg/agent/ebpfspy/bpf/libs/bcc-syms/lib:$(pyroscope_dir)/pkg/agent/ebpfspy/bpf/libs/libbpf/lib64/
# run
./bin/pyroscope connect --spy-name=ebpfspy --pid=-1 --detect-subprocesses=false
```
