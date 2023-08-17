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

The [Connect protocol][protocol] is a simple protocol that works over HTTP/1.1
or HTTP/2. It takes the best portions of gRPC and gRPC-Web, including
streaming, and packages them into a protocol that works equally well in
browsers, monoliths, and microservices. Calling a Connect API is as easy as
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
Instead of cURL, we could call our API with `buf curl`:

```
go install github.com/bufbuild/buf/cmd/buf@latest
buf curl --protocol grpc \
    --data '{"sentence": "I feel happy."}' \
    https://demo.connect.build/buf.connect.demo.eliza.v1.ElizaService/Say
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
  "context"
  "log"
  "net/http"

  "github.com/bufbuild/connect-go"
  pingv1 "github.com/bufbuild/connect-go/internal/gen/connect/ping/v1"
  "github.com/bufbuild/connect-go/internal/gen/connect/ping/v1/pingv1connect"
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
  mux.Handle(pingv1connect.NewPingServiceHandler(&PingServer{}))
  err := http.ListenAndServe(
    "localhost:8080",
    // For gRPC clients, it's convenient to support HTTP/2 without TLS. You can
    // avoid x/net/http2 by using http.ListenAndServeTLS.
    h2c.NewHandler(mux, &http2.Server{}),
  )
  log.Fatalf("listen failed: %v", err)
}
```

With that server running, you can make requests with any gRPC or Connect
client. To write a client using `connect-go`,

```go
package main

import (
  "context"
  "log"
  "net/http"

  "github.com/bufbuild/connect-go"
  pingv1 "github.com/bufbuild/connect-go/internal/gen/connect/ping/v1"
  "github.com/bufbuild/connect-go/internal/gen/connect/ping/v1/pingv1connect"
)

func main() {
  client := pingv1connect.NewPingServiceClient(
    http.DefaultClient,
    "http://localhost:8080/",
  )
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
* [connect-demo]: service powering demo.connect.build, including bidi streaming
* [connect-web]: TypeScript clients for web browsers
* [Buf Studio]: web UI for ad-hoc RPCs
* [connect-crosstest]: gRPC and gRPC-Web interoperability tests

## Status: Stable

This module is stable. It supports:

* The [two most recent major releases][go-support-policy] of Go.
* [APIv2] of Protocol Buffers in Go (`google.golang.org/protobuf`).

Within those parameters, `connect-go` follows semantic versioning. We will
_not_ make breaking changes in the 1.x series of releases.

## Legal

Offered under the [Apache 2 license][license].

[APIv2]: https://blog.golang.org/protobuf-apiv2
[Buf Studio]: https://studio.buf.build/
[Getting Started]: https://connect.build/docs/go/getting-started
[blog]: https://buf.build/blog/connect-a-better-grpc
[connect-crosstest]: https://github.com/bufbuild/connect-crosstest
[connect-demo]: https://github.com/bufbuild/connect-demo
[connect-grpchealth-go]: https://github.com/bufbuild/connect-grpchealth-go
[connect-grpcreflect-go]: https://github.com/bufbuild/connect-grpcreflect-go
[connect-web]: https://www.npmjs.com/package/@bufbuild/connect-web
[demo]: https://github.com/bufbuild/connect-demo
[docs-deployment]: https://connect.build/docs/go/deployment
[docs]: https://connect.build
[go-support-policy]: https://golang.org/doc/devel/release#policy
[health checks]: https://github.com/bufbuild/connect-grpchealth-go
[license]: https://github.com/bufbuild/connect-go/blob/main/LICENSE
[protobuf]: https://developers.google.com/protocol-buffers
[protocol]: https://connect.build/docs/protocol
[server reflection]: https://github.com/bufbuild/connect-grpcreflect-go
