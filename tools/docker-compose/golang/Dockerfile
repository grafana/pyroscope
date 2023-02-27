FROM golang as builder

WORKDIR /app

# Copy the source-code and Compile the code.
COPY main.go .
COPY go.mod .
RUN CGO_ENABLED=0 GOOS=linux go build

FROM alpine:3.17.2

# Copy over the directory containing the compiled binary for the profiling.
COPY --from=builder /app/golang /golang
RUN ls -al

EXPOSE 6060
# Run the application when the docker image is run, using either CMD (as is done
# here) or ENTRYPOINT.
CMD ./golang
