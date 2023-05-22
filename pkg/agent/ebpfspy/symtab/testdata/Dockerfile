FROM --platform=linux/amd64 ubuntu:22.04 as builder

RUN apt-get update && apt-get -y install gcc make

ADD src.c docker.sh ./
RUN bash docker.sh

FROM scratch
COPY --from=builder elf elf.debug elf.stripped elf.debuglink elf.nopie ./elfs/
COPY --from=builder /usr/lib/debug/ ./usr/lib/debug/
COPY --from=builder /usr/lib/x86_64-linux-gnu/libc.so.6 ./usr/lib/x86_64-linux-gnu/libc.so.6
COPY --from=builder /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2 ./usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
