.DEFAULT_GOAL := profile.bpf.o

CLANG ?= clang
BPFTOOL ?= bpftool
CFLAGS = -ggdb -gdwarf -O2 -Wall -fpie -Wno-unused-variable -Wno-unused-function
#LDFLAGS =

#GOARCH := amd64

#CGO_CFLAGS_STATIC = ""





profile.bpf.o: profile.bpf.c vmlinux.h
	$(CLANG) $(CFLAGS) -target bpf -D__TARGET_ARCH_x86 -I. -I/home/korniltsev/github/libbpf/src/build/include -c profile.bpf.c -o $@


vmlinux.h:
	$(BPFTOOL) btf dump file /sys/kernel/btf/vmlinux format c > $@

#CGO_LDFLAGS_STATIC = "-lelf -lz -lbpf"
#CGO_EXTLDFLAGS_STATIC = '-w -extldflags "-static"'
#main: main.go profile.bpf.o
#	CC=$(CLANG) \
#		CGO_CFLAGS=$(CGO_CFLAGS_STATIC) \
#		CGO_LDFLAGS=$(CGO_LDFLAGS_STATIC) \
#                GOARCH=$(GOARCH) \
#                go build \
#                -tags netgo -ldflags $(CGO_EXTLDFLAGS_STATIC) \
#                -o main ./main.go

clean:
	rm profile.bpf.o include/vmlinux.h
	#rm main