FROM golang:1.22.11

WORKDIR /go/src/app
COPY . .
RUN go build main.go
CMD ["./main"]
