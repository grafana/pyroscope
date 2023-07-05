FROM --platform=linux/amd64 ubuntu:22.04 as builder

RUN apt-get update && apt-get -y install gcc make

ADD src.c lib.c docker.sh ./
RUN bash docker.sh


FROM --platform=linux/amd64 golang:1.2 as go12
ADD hello.go hello.go
RUN go build hello.go
RUN go build -ldflags="-extldflags=-static" -o hello-static hello.go

FROM --platform=linux/amd64 golang:1.16 as go116
ADD hello.go hello.go
RUN go build hello.go
RUN go build -ldflags="-extldflags=-static -T 4294963200" -o hello-static hello.go

FROM --platform=linux/amd64 golang:1.18 as go118
ADD hello.go hello.go
RUN go build hello.go
RUN go build -ldflags="-extldflags=-static -T 1099511623680" -o hello-static hello.go

FROM --platform=linux/amd64 golang:1.20 as go120
ADD hello.go hello.go
RUN go build hello.go
RUN go build -ldflags="-extldflags=-static" -o hello-static hello.go

FROM scratch
COPY --from=builder elf elf.debug elf.stripped elf.debuglink elf.nopie elf.nobuildid libexample.so ./elfs/
COPY --from=builder /usr/lib/debug/ ./usr/lib/debug/
COPY --from=go12 /go/hello ./elfs/go12
COPY --from=go116 /go/hello ./elfs/go16
COPY --from=go118 /go/hello ./elfs/go18
COPY --from=go120 /go/hello ./elfs/go20
COPY --from=go12 /go/hello-static ./elfs/go12-static
COPY --from=go116 /go/hello-static ./elfs/go16-static
COPY --from=go118 /go/hello-static ./elfs/go18-static
COPY --from=go120 /go/hello-static ./elfs/go20-static
