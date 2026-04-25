# keda-launcher-scaler

`keda-launcher-scaler` is a KEDA external scaler that activates a ScaledObject for
time-bounded launch requests received over HTTP.

The service accepts request windows through `POST /requests`, routes them by
ScaledObject, and exposes the KEDA external scaler gRPC API.

The HTTP API contract is defined by `internal/common/contracts/receivers/http/openapi.yaml`.
Transport-agnostic client domain types are published from `pkg/client`, and the HTTP client implementation plus generated OpenAPI types are published from `pkg/client/http`.

## Usage

Run the scaler locally:

```sh
go run ./cmd/keda-launcher-scaler
```

By default, the HTTP receiver listens on `:8080` and the gRPC external scaler
listens on `:9090`.

Run the sample client locally:

```sh
SCALED_OBJECT_NAMESPACE=default \
SCALED_OBJECT_NAME=worker \
REQUEST_INTERVAL=30s \
REQUEST_DURATION=1m \
go run ./cmd/keda-launcher-client
```

`cmd/keda-launcher-client` is a sample binary that uses the HTTP client implementation in `pkg/client/http`.

For library use, `pkg/client` defines the transport-agnostic domain types and
client interface, while `pkg/client/http` provides the HTTP implementation.
Detailed API usage is documented in the Go source comments.

Example request:

```sh
curl -X POST http://localhost:8080/requests \
  -H 'content-type: application/json' \
  -d '{
    "requestId": "example-request",
    "scaledObject": {
      "namespace": "default",
      "name": "worker"
    },
    "duration": "5m"
  }'
```

## Container

Build the image:

```sh
docker build -f Dockerfile.server -t keda-launcher-scaler .
docker build -f Dockerfile.client -t keda-launcher-client .
```

Run the container:

```sh
docker run --rm \
  -p 8080:8080 \
  -p 9090:9090 \
  keda-launcher-scaler
```

## Configuration

Configuration is loaded from environment variables.

| Variable | Default | Description |
| --- | --- | --- |
| `SERVICE_NAME` | `keda-launcher-scaler` | OpenTelemetry service name. |
| `HTTP_LISTEN_ADDRESS` | `:8080` | HTTP receiver listen address. |
| `GRPC_LISTEN_ADDRESS` | `:9090` | KEDA external scaler gRPC listen address. |
| `SHUTDOWN_TIMEOUT` | `10s` | Graceful shutdown timeout. |
| `LOG_LEVEL` | `info` | Log level. |
| `REQUEST_BUFFER_SIZE` | `128` | Buffered request queue size. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | empty | OTLP trace exporter endpoint. |
| `OTEL_EXPORTER_OTLP_INSECURE` | `true` | Whether to use an insecure OTLP connection. |

Client configuration:

| Variable | Default | Description |
| --- | --- | --- |
| `SERVICE_NAME` | `keda-launcher-client` | OpenTelemetry service name. |
| `RECEIVER_URL` | `http://localhost:8080` | Scaler HTTP receiver base URL. |
| `REQUEST_ID` | derived from scaled object | Optional request ID override. |
| `SCALED_OBJECT_NAMESPACE` | required | Target ScaledObject namespace. |
| `SCALED_OBJECT_NAME` | required | Target ScaledObject name. |
| `REQUEST_INTERVAL` | required | Interval for sending launch requests. |
| `REQUEST_DURATION` | required | Duration sent in each launch request. Must be greater than `REQUEST_INTERVAL`. |
| `SHUTDOWN_TIMEOUT` | `10s` | Graceful shutdown timeout. |
| `LOG_LEVEL` | `info` | Log level. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | empty | OTLP trace exporter endpoint. |
| `OTEL_EXPORTER_OTLP_INSECURE` | `true` | Whether to use an insecure OTLP connection. |

## License

MIT
