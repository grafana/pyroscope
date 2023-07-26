LIBBPF_VERSION=0.6.1
LIBBPF_PREFIX="https://raw.githubusercontent.com/libbpf/libbpf/v$(LIBBPF_VERSION)"

.PHONY: get-headers
get-headers: get-libbpf-headers get-vmlinux-header

.PHONY: get-libbpf-headers
get-libbpf-headers:
	cd libbpf && \
		curl -O $(LIBBPF_PREFIX)/src/bpf_endian.h && \
		curl -O $(LIBBPF_PREFIX)/src/bpf_helper_defs.h && \
		curl -O $(LIBBPF_PREFIX)/src/bpf_helpers.h && \
		curl -O $(LIBBPF_PREFIX)/src/bpf_tracing.h && \
		curl -O $(LIBBPF_PREFIX)/src/bpf_core_read.h

.PHONY: get-vmlinux-header
get-vmlinux-header:
	cd vmlinux && \
		curl -o vmlinux.h https://raw.githubusercontent.com/iovisor/bcc/v0.27.0/libbpf-tools/x86/vmlinux_518.h

.PHONY: clean
clean:
	rm -rf libbpf/*.h vmlinux/*.h

.PHONY: profile.so
profile.so:
	clang -DBPF_NO_PRESERVE_ACCESS_INDEX -DPYROSCOPE_UME -D __TARGET_ARCH_x86 \
 		-O0 -g3 -Wall -fpie -fPIC -nostdinc -std=gnu11 \
 		-fno-omit-frame-pointer \
 		-shared \
 		-Wno-unused-variable -Wno-unused-function \
 		-I./libbpf -I./vmlinux/ \
 		-o ../profile_ume_x86.so  profile.bpf.c
