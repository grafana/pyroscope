FROM golang:1.21.10 AS build

WORKDIR /app
COPY main.go .
RUN CGO_ENABLED=0 GOOS=linux go build -o collect-ebpf-sidecar ./main.go

FROM alpine:3.19.1

# Copy the binary from the build stage
COPY --from=build /app/collect-ebpf-sidecar /usr/local/bin/

# Set the timezone and install CA certificates
RUN apk --no-cache add ca-certificates tzdata

# Set the entrypoint command
ENTRYPOINT ["/usr/local/bin/collect-ebpf-sidecar"]
