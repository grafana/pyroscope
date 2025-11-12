FROM golang:1.24.9

WORKDIR /go/src/app
COPY . .
RUN go build main.go
CMD ["./main"]
