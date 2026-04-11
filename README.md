# keda-launcher-scaler

`keda-launcher-scaler` is a KEDA external scaler that activates a ScaledObject for
time-bounded launch requests received over HTTP.

The service accepts request windows through `POST /requests`, routes them by
ScaledObject, and exposes the KEDA external scaler gRPC API.

## Usage

Run the scaler locally:

```sh
go run .
```

By default, the HTTP receiver listens on `:8080` and the gRPC external scaler
listens on `:9090`.

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
docker build -t keda-launcher-scaler .
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

## License

MIT
