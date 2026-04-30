# Deployment Standards

このプロジェクトは server/client の二つの Go バイナリを別々の container image として build/publish します。release は Git tag を起点にし、Docker Hub へ publish します。

## Artifacts

- server image: `keda-launcher-scaler`
- client image: `keda-launcher-client`
- server Dockerfile: `Dockerfile.server`
- client Dockerfile: `Dockerfile.client`

server は `./cmd/keda-launcher-scaler`、client は `./cmd/keda-launcher-client` を build します。新しいバイナリを追加する場合も、`cmd/<name>` と Dockerfile/workflow matrix の対応を明確にします。

## Container Build

Dockerfile は multi-stage build を使います。

- builder は Go toolchain image を使う。
- `go.mod` / `go.sum` を先に copy し、module download cache を活かす。
- `CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w"` で静的寄りの小さい binary を作る。
- runtime は distroless nonroot を使う。
- runtime image には build artifact だけを copy する。

server image は HTTP `8080` と gRPC `9090` を expose します。client image は outbound HTTP client なので port expose を持ちません。

## CI

Go test workflow は Go 関連差分に反応し、`go-version-file: go.mod` で toolchain version を揃えます。

```sh
go test ./...
```

CI に追加する検証は、まず repo-owned behavior を守るものに限定します。生成コード更新の検出、race detector、docker build check などは、変更のリスクに応じて追加します。

## Release Flow

Docker publish は `v*.*.*` tag push で実行します。GitHub Actions の matrix で server/client の image と Dockerfile を対応させます。

```text
v1.2.3 tag
  -> Dockerfile.server -> <dockerhub-user>/keda-launcher-scaler:v1.2.3
  -> Dockerfile.client -> <dockerhub-user>/keda-launcher-client:v1.2.3
```

`latest` を安定運用の前提にしないでください。Kubernetes manifests や利用側では semver tag を pin することを優先します。

## Configuration

runtime configuration は environment variables から渡します。secret は image に焼き込まず、Kubernetes Secret や CI secret store から注入します。

server の主要設定:

- `HTTP_LISTEN_ADDRESS`
- `GRPC_LISTEN_ADDRESS`
- `SHUTDOWN_TIMEOUT`
- `LOG_LEVEL`
- `REQUEST_BUFFER_SIZE`
- `OTEL_EXPORTER_OTLP_ENDPOINT`
- `OTEL_EXPORTER_OTLP_INSECURE`

client の主要設定:

- `RECEIVER_URL`
- `REQUEST_ID`
- `SCALED_OBJECT_NAMESPACE`
- `SCALED_OBJECT_NAME`
- `REQUEST_INTERVAL`
- `REQUEST_DURATION`

## Runtime Behavior

server は SIGINT/SIGTERM を受けると graceful shutdown timeout 内で receiver、arbitrator router、gRPC scaler server を停止します。Kubernetes に配置する場合は、termination grace period が `SHUTDOWN_TIMEOUT` より短くならないようにします。

HTTP receiver は readiness endpoint を持っていません。運用側で probe を追加する場合は、実装を伴う endpoint を先に追加し、単なる TCP check だけで domain readiness を表現しないようにします。

## Observability

container は JSON log を stdout に出します。trace export は `OTEL_EXPORTER_OTLP_ENDPOINT` が設定された場合だけ有効です。collector の endpoint と TLS/insecure 設定は環境ごとに注入します。

ログや trace attribute に secret を入れないでください。request correlation には `requestId` と `scaledObject.namespace/name` を使います。

