# API Standards

このプロジェクトの外部契約は、HTTP receiver API と KEDA external scaler gRPC API の二つです。HTTP contract は OpenAPI YAML を source of truth とし、gRPC contract は KEDA が提供する external scaler protobuf に従います。

## Contract Source

HTTP API の正は `internal/common/contracts/receivers/http/openapi.yaml` です。server/client の Go 型は `oapi-codegen` で同じ YAML から生成します。

```text
internal/common/contracts/receivers/http/openapi.yaml
  -> internal/server/receiver/http/openapi.gen.go
  -> pkg/client/http/openapi.gen.go
```

HTTP の request/response schema を変える場合は、YAML を先に更新してから `go generate ./...` で生成コードを更新します。生成コードを手で直して契約変更を表現してはいけません。

## HTTP Endpoint

現在の receiver API は `POST /requests` です。これは「指定した ScaledObject を時間付きで active にする request window を受け付ける」ための command endpoint です。

成功時は処理完了ではなく受理を表すため `202 Accepted` を返します。

```json
{
  "requestId": "example-request",
  "scaledObject": {
    "namespace": "default",
    "name": "worker"
  },
  "duration": "5m"
}
```

## Request Shape

`LaunchRequest` は以下を基本とします。

- `requestId` は必須。後続 request で同じ ID を使うと同じ window を更新する。
- `scaledObject.namespace` と `scaledObject.name` は必須。
- `startAt` は任意。未指定なら受理時刻を使う。
- `duration` と `endAt` はどちらか一方だけを指定する。
- `duration` は Go duration 文字列として解釈する。
- schema にない property は拒否する。

HTTP layer は JSON schema validation と wire 変換を担当し、時間 window の domain rule は `receiver.NormalizeRequest` で評価します。

## Response Shape

受理時は実際に使われる window を返します。

```json
{
  "requestId": "example-request",
  "scaledObject": {
    "namespace": "default",
    "name": "worker"
  },
  "effectiveStart": "2026-04-22T10:00:00Z",
  "effectiveEnd": "2026-04-22T10:05:00Z"
}
```

クライアントが知るべき時刻は、入力値そのものではなく `effectiveStart` / `effectiveEnd` です。

## Error Shape

HTTP API error は現行 contract に合わせて単純な `message` を返します。

```json
{ "message": "duration must be positive" }
```

新しい error field を追加する場合は、OpenAPI schema と generated client/server の両方を更新し、既存利用者への互換性を確認します。

## KEDA gRPC API

gRPC 側は KEDA external scaler の interface を実装します。

- `IsActive` は現在の active state を返す。
- `StreamIsActive` は購読 channel から active state update を stream する。
- `GetMetricSpec` は `keda-launcher-active` を target `1` の metric として返す。
- `GetMetrics` は active なら `1`、inactive なら `0` を返す。

`ScaledObjectRef` の `namespace` と `name` が空の場合は `codes.InvalidArgument` を返します。

## Client Boundary

HTTP client は generated client を直接公開せず、`pkg/client` の domain type に変換します。

```go
receiver, err := httpclient.New(receiverURL, httpclient.WithHTTPClient(httpClient))
accepted, err := receiver.Launch(ctx, client.LaunchRequest{...})
```

HTTP status や error body の詳細は `pkg/client/http.Error` に閉じ込め、呼び出し側は `errors.As` で必要な場合だけ参照します。

