Connect
=======

[![Build](https://github.com/bufbuild/connect-go/actions/workflows/ci.yaml/badge.svg?branch=main)](https://github.com/bufbuild/connect-go/actions/workflows/ci.yaml)
[![Report Card](https://goreportcard.com/badge/github.com/bufbuild/connect-go)](https://goreportcard.com/report/github.com/bufbuild/connect-go)
[![GoDoc](https://pkg.go.dev/badge/github.com/bufbuild/connect-go.svg)](https://pkg.go.dev/github.com/bufbuild/connect-go)

Connect is a slim library for building browser and gRPC-compatible HTTP APIs.
You write a short [Protocol Buffer][protobuf] schema and implement your
application logic, and Connect generates code to handle marshaling, routing,
compression, and content type negotiation. It also generates an idiomatic,
type-safe client. Handlers and clients support three protocols: gRPC, gRPC-Web,
and Connect's own protocol.

The [Connect protocol][protocol] is a simple, POST-only protocol that works
over HTTP/1.1 or HTTP/2. It takes the best portions of gRPC and gRPC-Web,
including streaming, and packages them into a protocol that works equally well
in browsers, monoliths, and microservices. Calling a Connect API is as easy as
using `curl`. Try it with our live demo:

```
curl \
    --header "Content-Type: application/json" \
    --data '{"sentence": "I feel happy."}' \
    https://demo.connect.build/buf.connect.demo.eliza.v1.ElizaService/Say
```

Handlers and clients also support the gRPC and gRPC-Web protocols, including
streaming, headers, trailers, and error details. gRPC-compatible [server
reflection][] and [health checks][] are available as standalone packages.
Instead of cURL, we could call our API with `grpcurl`:

```
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
grpcurl \
    -d '{"sentence": "I feel happy."}' \
    demo.connect.build:443 \
    buf.connect.demo.eliza.v1.ElizaService/Say
```

Under the hood, Connect is just [Protocol Buffers][protobuf] and the standard
library: no custom HTTP implementation, no new name resolution or load
balancing APIs, and no surprises. Everything you already know about `net/http`
still applies, and any package that works with an `http.Server`, `http.Client`,
or `http.Handler` also works with Connect.

For more on Connect, see the [announcement blog post][blog], the documentation
on [connect.build][docs] (especially the [Getting Started] guide for Go), the
[demo service][demo], or the [protocol specification][protocol].

## A small example

Curious what all this looks like in practice? From a [Protobuf
schema](internal/proto/connect/ping/v1/ping.proto), we generate [a small RPC
package](internal/gen/connect/ping/v1/pingv1connect/ping.connect.go). Using that
package, we can build a server:

```go
package main

import (
  "log"
  "net/http"

  "github.com/bufbuild/connect-go"
  "github.com/bufbuild/connect-go/internal/gen/connect/connect/ping/v1/pingv1connect"
  pingv1 "github.com/bufbuild/connect-go/internal/gen/go/connect/ping/v1"
  "golang.org/x/net/http2"
  "golang.org/x/net/http2/h2c"
)

type PingServer struct {
  pingv1connect.UnimplementedPingServiceHandler // returns errors from all methods
}

func (ps *PingServer) Ping(
  ctx context.Context,
  req *connect.Request[pingv1.PingRequest],
) (*connect.Response[pingv1.PingResponse], error) {
  // connect.Request and connect.Response give you direct access to headers and
  // trailers. No context-based nonsense!
  log.Println(req.Header().Get("Some-Header"))
  res := connect.NewResponse(&pingv1.PingResponse{
    // req.Msg is a strongly-typed *pingv1.PingRequest, so we can access its
    // fields without type assertions.
    Number: req.Msg.Number,
  })
  res.Header().Set("Some-Other-Header", "hello!")
  return res, nil
}

func main() {
  mux := http.NewServeMux()
  // The generated constructors return a path and a plain net/http
  // handler.
  mux.Handle(pingv1.NewPingServiceHandler(&PingServer{}))
  http.ListenAndServe(
    "localhost:8080",
    // For gRPC clients, it's convenient to support HTTP/2 without TLS. You can
    // avoid x/net/http2 by using http.ListenAndServeTLS.
    h2c.NewHandler(mux, &http2.Server{}),
  )
}
```

With that server running, you can make requests with any gRPC or Connect
client. To write a client using `connect-go`,

```go
package main

import (
  "log"
  "net/http"

  "github.com/bufbuild/connect-go"
  "github.com/bufbuild/connect-go/internal/gen/connect/connect/ping/v1/pingv1connect"
  pingv1 "github.com/bufbuild/connect-go/internal/gen/go/connect/ping/v1"
)

func main() {
  client, err := pingv1connect.NewPingServiceClient(
    http.DefaultClient,
    "https://localhost:8080/",
  )
  if err != nil {
    log.Fatalln(err)
  }
  req := connect.NewRequest(&pingv1.PingRequest{
    Number: 42,
  })
  req.Header().Set("Some-Header", "hello from connect")
  res, err := client.Ping(context.Background(), req)
  if err != nil {
    log.Fatalln(err)
  }
  log.Println(res.Msg)
  log.Println(res.Header().Get("Some-Other-Header"))
}
```

Of course, `http.ListenAndServe` and `http.DefaultClient` aren't fit for
production use! See Connect's [deployment docs][docs-deployment] for a guide to
configuring timeouts, connection pools, observability, and h2c.

## Ecosystem

* [connect-grpchealth-go]: gRPC-compatible health checks
* [connect-grpcreflect-go]: gRPC-compatible server reflection
* [connect-demo]: demonstration service powering demo.connect.build, including bidi streaming
* [connect-crosstest]: gRPC and gRPC-Web interoperability tests

## Status

This module is a beta: we rely on it in production, but we may make a few
changes as we gather feedback from early adopters. We're planning to tag a
stable v1 in October, soon after the Go 1.19 release.

## Support and versioning

`connect-go` supports:

* The [two most recent major releases][go-support-policy] of Go, with a minimum
  of Go 1.18.
* [APIv2] of Protocol Buffers in Go (`google.golang.org/protobuf`).

Within those parameters, Connect follows semantic versioning.

## Legal

Offered under the [Apache 2 license][license].

[APIv2]: https://blog.golang.org/protobuf-apiv2
[Getting Started]: https://connect.build/go/getting-started
[blog]: https://buf.build/blog/connect-a-better-grpc
[connect-grpchealth-go]: https://github.com/bufbuild/connect-grpchealth-go
[connect-grpcreflect-go]: https://github.com/bufbuild/connect-grpcreflect-go
[connect-demo]: https://github.com/bufbuild/connect-demo
[connect-crosstest]: https://github.com/bufbuild/connect-crosstest
[demo]: https://github.com/bufbuild/connect-demo
[docs]: https://connect.build
[docs-deployment]: https://connect.build/docs/go/deployment
[go-support-policy]: https://golang.org/doc/devel/release#policy
[license]: https://github.com/bufbuild/connect-go/blob/main/LICENSE.txt
[protobuf]: https://developers.google.com/protocol-buffers
[protocol]: https://connect.build/docs/protocol
[server reflection]: https://github.com/bufbuild/connect-grpcreflect-go
[health checks]: https://github.com/bufbuild/connect-grpchealth-go
