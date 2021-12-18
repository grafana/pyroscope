FROM golang:1.17

RUN mkdir /app
WORKDIR /app

ADD main.go main.go
ADD fixtures fixtures

RUN go mod init app
RUN go get github.com/pyroscope-io/pyroscope
RUN go get google.golang.org/protobuf/proto
RUN go mod tidy
CMD go run main.go
