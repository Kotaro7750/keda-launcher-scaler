# Error Handling Standards

このプロジェクトでは、境界ごとに error の表現を変えます。内部では wrap された Go error、HTTP では OpenAPI の `ErrorResponse`、gRPC では `status.Error`、CLI entrypoint では stderr の fatal message を使います。

## Internal Errors

内部処理の失敗は `fmt.Errorf("operation: %w", err)` で文脈を付けて返します。

```go
cfg, err := config.Load()
if err != nil {
	return fmt.Errorf("load config: %w", err)
}
```

wrap する文脈は「何をしようとして失敗したか」を短く書きます。呼び出し元が `errors.Is` / `errors.As` で扱う可能性がある error は `%w` で保持します。

## Validation Errors

domain validation は、入力を正規化する層で行います。

- HTTP schema validation は OpenAPI middleware に任せる。
- request window の business rule は `receiver.NormalizeRequest` に集約する。
- config validation は各 `internal/*/config.Load()` で完了させる。

validation message は利用者が修正できる形にします。

```go
return Config{}, fmt.Errorf("REQUEST_DURATION: must be greater than REQUEST_INTERVAL")
```

## HTTP Errors

HTTP receiver では、利用者入力の問題は `echo.NewHTTPError` で 4xx に変換します。

- schema/domain validation failure: `400 Bad Request`
- request context が受理前に cancel された場合: `408 Request Timeout`

HTTP response body は OpenAPI の `ErrorResponse` に合わせて `message` を持ちます。秘密情報、内部 address、stack trace は response に含めません。

## HTTP Client Errors

`pkg/client/http` は非 202 response を `*httpclient.Error` として返します。

```go
var apiErr *httpclient.Error
if errors.As(err, &apiErr) {
	// apiErr.StatusCode / apiErr.Message
}
```

generated response の body は trim して `Message` に入れます。transport 失敗は `post request: %w` として wrap し、API response error と区別できるようにします。

## gRPC Errors

KEDA external scaler API では、request が不正な場合 `status.Error(codes.InvalidArgument, "...")` を返します。

```go
if ref.GetNamespace() == "" || ref.GetName() == "" {
	return types.ScaledObjectKey{}, status.Error(codes.InvalidArgument, "name and namespace are required")
}
```

KEDA から渡される `ScaledObjectRef` の不足は caller error として扱い、内部 panic にしません。

## Lifecycle Errors

`Run(ctx)` は context cancellation を明示的に扱います。

- expected shutdown は nil または `ctx.Err()` のいずれか、既存 component contract に合わせる。
- server close の既知 error は正常終了として扱う。
- unexpected component error は上位で log して process を終了する。

HTTP server の `http.ErrServerClosed`、gRPC server の `grpc.ErrServerStopped` は通常の shutdown として扱います。

## Logging

logger は JSON `slog` を使います。error log は operation、error、requestId、scaledObject など調査に必要な context を key-value で持たせます。

```go
logger.Error(
	"Launch request failed",
	"error", err,
	"requestId", request.RequestID,
	"scaledObject.namespace", request.ScaledObject.Namespace,
	"scaledObject.name", request.ScaledObject.Name,
)
```

token、credential、secret、full request body は log に出しません。

