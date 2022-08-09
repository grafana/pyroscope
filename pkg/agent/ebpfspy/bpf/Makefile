CLANG ?= clang

CFLAGS := $(CFLAGS) -ggdb -gdwarf -O2 -Wall -fpie -Wno-unused-variable -Wno-unused-function


libbpf-src:
	wget https://github.com/libbpf/libbpf/archive/refs/tags/v0.8.1.tar.gz
	tar -xvf v0.8.1.tar.gz
	mv libbpf-0.8.1 libbpf-src
	rm v0.8.1.tar.gz

libs/libbpf: libbpf-src
	PREFIX=$(shell pwd)/libs/libbpf make -C libbpf-src/src -j16 install

bcc-src:
	git clone https://github.com/korniltsev/bcc.git bcc-src
	cd bcc-src && git checkout b9554b585afe18540ba98dde5b667e5b4036f479

libs/bcc-syms: bcc-src
	cd bcc-src \
		&& mkdir build \
		&& cd build \
		&& cmake ../build-syms  -DCMAKE_C_COMPILER=clang -DCMAKE_CXX_COMPILER=clang++ -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX=$(shell pwd)/libs/bcc-syms \
		&& make -j16 install

profile.bpf.o: profile.bpf.c profile.bpf.h vmlinux.h libs/libbpf
	$(CLANG) $(CFLAGS) -target bpf -D__TARGET_ARCH_x86 -I. -Ilibs/libbpf/include -c profile.bpf.c -o $@

vmlinux.h:
	wget https://raw.githubusercontent.com/iovisor/bcc/798a1056903b57687fd9d30a43c64c7a4402934c/libbpf-tools/x86/vmlinux_518.h -O $@

clean:
	rm -rf profile.bpf.o vmlinux.h libbpf* *.tar.gz bcc-src libs
