FROM golang:1.25.13 AS build

WORKDIR /go/src/app
COPY . .
# loadgen.go shares this directory, so the build has to name the file.
RUN CGO_ENABLED=0 go build -o main main.go

FROM gcr.io/distroless/static-debian12:nonroot

# Path must match: the deployment invokes the binary relative to this workdir.
WORKDIR /go/src/app
COPY --from=build /go/src/app/main .
CMD ["./main"]
