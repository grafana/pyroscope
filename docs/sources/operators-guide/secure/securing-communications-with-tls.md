---
aliases:
  - /docs/mimir/latest/operators-guide/securing/securing-communications-with-tls/
description: Learn how to configure TLS between Grafana Mimir components.
menuTitle: Securing communications with TLS
title: Securing Grafana Mimir communications with TLS
weight: 50
---

# Securing Grafana Mimir communications with TLS

Grafana Mimir is a distributed system with significant traffic between its components.
To allow for secure communication, Grafana Mimir supports TLS between its
components. This topic describes the process you use to set up TLS.

### Generation of certificates to configure TLS

To establish secure inter-component communication in Grafana Mimir with TLS, you must generate certificates using a certificate authority (CA).
The CA should be private to the organization because certificates signed by the CA will have permissions to communicate with the cluster.

> **Note**: The generated certficates are valid for 100,000 days. You can change the duration by adjusting the `-days` option in the command. We recommended that you replace the certificates every two years.

The following script generates self-signed certificates for the cluster.
The script generates private keys `client.key`, `server.key` and certificates `client.crt`, `server.crt` for both the client and server.
The script generates the CA cert as `root.crt`.

```
# keys
openssl genrsa -out root.key
openssl genrsa -out client.key
openssl genrsa -out server.key

# root cert / certifying authority
openssl req -x509 -new -nodes -key root.key -subj "/C=US/ST=KY/O=Org/CN=root" -sha256 -days 100000 -out root.crt

# csrs - certificate signing requests
openssl req -new -sha256 -key client.key -subj "/C=US/ST=KY/O=Org/CN=client" -out client.csr
openssl req -new -sha256 -key server.key -subj "/C=US/ST=KY/O=Org/CN=localhost" -out server.csr

# certificates
openssl x509 -req -in client.csr -CA root.crt -CAkey root.key -CAcreateserial -out client.crt -days 100000 -sha256
openssl x509 -req -in server.csr -CA root.crt -CAkey root.key -CAcreateserial -out server.crt -days 100000 -sha256
```

### Configure TLS certificates in Grafana Mimir

Every gRPC link between Grafana Mimir components supports TLS configuration as specified in server flags and client flags.

#### Server flags

Server flag settings determine if a server requires a client to provide a valid certificate back to the server.
The flags support all the values defined in the [crypto/tls](https://pkg.go.dev/crypto/tls#ClientAuthType) standard library.

For all values except `NoClientCert`, the policy defines that the server requests a client certificate during the handshake. The values determine whether the client must send certificates and if the server must verify them.

Use the following options to define the server certificate policy:

- `NoClientCert`: The server does not request a client certificate.
- `RequestClientCert`: The server requests a client certificate, but the client is not required to send it.
- `RequireClientCert`: The server requests a client to send at least one certificate, but a valid certificate is not required.
- `VerifyClientCertIfGiven`: The server does not require the client to send a certificate, but if it does, the certificate must be valid.
- `RequireAndVerifyClientCert`: The server requires the client to send at least one valid certificate.

In the following example, both of the server authorization flags, `-server.http-tls-client-auth` and `-server.grpc-tls-client-auth`, are shown with the most restrictive option, which is `RequiredAndVerifyClientCert`.

```
    # Path to the TLS Cert for the HTTP Server
    -server.http-tls-cert-path=/path/to/server.crt

    # Path to the TLS Key for the HTTP Server
    -server.http-tls-key-path=/path/to/server.key

    # Type of Client Auth for the HTTP Server
    -server.http-tls-client-auth="RequireAndVerifyClientCert"

    # Path to the Client CA Cert for the HTTP Server
    -server.http-tls-ca-path="/path/to/root.crt"

    # Path to the TLS Cert for the gRPC Server
    -server.grpc-tls-cert-path=/path/to/server.crt

    # Path to the TLS Key for the gRPC Server
    -server.grpc-tls-key-path=/path/to/server.key

    # Type of Client Auth for the gRPC Server
    -server.grpc-tls-client-auth="RequireAndVerifyClientCert"

    # Path to the Client CA Cert for the gRPC Server
    -server.grpc-tls-ca-path=/path/to/root.crt
```

#### Client flags

You can configure TLS private keys, certificates, and CAs in a similar fashion for gRPC clients in Grafana Mimir.

To enable TLS for a component, use the client flag that contains the suffix `*.tls-enabled=true`, for example, `-querier.frontend-client.tls-enabled=true`.

The following Grafana Mimir components support TLS for inter-communication, which are shown with their corresponding configuration flag prefixes:

- Query scheduler gRPC client used to connect to query-frontends: `-query-scheduler.grpc-client-config.*`
- Querier gRPC client used to connect to store-gateways: `-querier.store-gateway-client.*`
- Query-frontend gRPC client used to connect to query-schedulers: `-query-frontend.grpc-client-config.*`
- Querier gRPC client used to connect to query-frontends and query-schedulers: `-querier.frontend-client.*`
- Ruler gRPC client used to connect to other ruler instances: `-ruler.client.*`
- Ruler gRPC client used to connect to query-frontend: `-ruler.query-frontend.grpc-client-config.*`
- Alertmanager gRPC client used to connect to other Alertmanager instances: `-alertmanager.alertmanager-client.*`
- gRPC client used by distributors, queriers, and rulers to connect to ingesters: `-ingester.client.*`
- etcd client used by all Mimir components to connect to etcd, which is required only if you're running the hash ring or HA tracker on the etcd backend: `-<prefix>.etcd.*`
- Memberlist client used by all Mimir components to gossip the hash ring, which is required only if you're running the hash ring on memberlist: `-memberlist.`

Each of the components listed above support the following TLS configuration options, which are shown with their corresponding flag suffixes:

- `*.tls-enabled=<boolean>`: Enable TLS in the client.
- `*.tls-server-name=<string>`: Override the expected name on the server certificate.
- `*.tls-insecure-skip-verify=<boolean>`: Skip validating the server certificate.

The following example shows how to configure the gRPC client flags in the querier used to connect to the query-frontend:

```
    # Path to the TLS Cert for the gRPC Client
    -querier.frontend-client.tls-cert-path=/path/to/client.crt

    # Path to the TLS Key for the gRPC Client
    -querier.frontend-client.tls-key-path=/path/to/client.key

    # Path to the TLS CA for the gRPC Client
    -querier.frontend-client.tls-ca-path=/path/to/root.crt
```
