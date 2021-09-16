FROM golang:1.16.3 as builder

WORKDIR /go/src/app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /go/bin/hotrod

FROM scratch
#EXPOSE 8080
COPY --from=builder /go/bin/hotrod /go/bin/hotrod

ENTRYPOINT ["/go/bin/hotrod"]
CMD ["all"]
