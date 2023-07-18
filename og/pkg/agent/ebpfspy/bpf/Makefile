CLANG ?= clang

CFLAGS := $(CFLAGS) -ggdb -gdwarf -O2 -Wall -fpie -Wno-unused-variable -Wno-unused-function


profile.bpf.o: profile.bpf.c profile.bpf.h vmlinux.h
	$(CLANG) $(CFLAGS) -target bpf -D__TARGET_ARCH_x86 -I. -c profile.bpf.c -o $@

vmlinux.h:
	wget https://raw.githubusercontent.com/iovisor/bcc/798a1056903b57687fd9d30a43c64c7a4402934c/libbpf-tools/x86/vmlinux_518.h -O $@

.PHONY: clean
clean:
	rm -rf profile.bpf.o vmlinux.h
