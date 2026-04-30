# Go Architecture Standards

このプロジェクトは、HTTP で受けた時間付き launch request を KEDA external scaler の gRPC API に反映する Go サービスです。実装は「起動バイナリ」「サーバ内部」「公開クライアント」「共通契約」を明確に分けます。

## パッケージ境界

- `cmd/keda-launcher-scaler` はサーババイナリの起動だけを持つ。
- `cmd/keda-launcher-client` はサンプル兼実運用可能な送信クライアントバイナリの起動だけを持つ。
- `internal/server` は receiver、arbitrator、KEDA scaler のサーバ内部実装を持つ。
- `internal/client` は client バイナリ専用の設定読み込みなど、公開 API にしない実装を持つ。
- `internal/common` は本当に server/client 間で共有する契約と observability に絞る。
- `pkg/client` は外部利用者向けの transport-neutral API とする。
- `pkg/client/http` は OpenAPI 生成クライアントを包む HTTP 実装とする。

新しい共有パッケージは、複数バイナリから実際に使われる安定した概念だけに限定します。単なる薄い wrapper や一箇所からしか使わない helper は、呼び出し元に置くことを優先します。

## 起動処理

バイナリ固有の bootstrap は `cmd/*/main.go` に直接置きます。設定読み込み、logger/tracer 初期化、signal context、graceful shutdown の組み立ては起動責務です。

```go
func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}
```

`run()` は失敗を `fmt.Errorf("operation: %w", err)` で包んで返し、`main()` は最終的なプロセス終了だけを担当します。

## サーバ構成

サーバは次の流れを保ちます。

```text
HTTP receiver -> requestCh -> arbitrator router -> per-ScaledObject arbitrator -> KEDA external scaler gRPC
```

- HTTP receiver は wire format を受け取り、transport-neutral な `receiver.RequestInput` に変換する。
- `receiver.NormalizeRequest` は request window の domain validation と時刻正規化を担当する。
- `arbitrator.Router` は `ScaledObjectKey` ごとに arbitrator を作り、購読者へ active state を配る。
- gRPC service は KEDA external scaler API を満たし、active state の問い合わせと stream を提供する。

receiver が直接 KEDA API を意識したり、gRPC service が HTTP payload を知ったりしないようにします。

## 公開クライアント API

`pkg/client` は transport-neutral な型と `Client` interface のみを公開します。

```go
type Client interface {
	Launch(ctx context.Context, req LaunchRequest) (AcceptedRequest, error)
}
```

HTTP 固有の生成型、status code、transport error は `pkg/client/http` に閉じ込めます。外部利用者に必要な型は、`pkg/client` の domain type を優先します。

## 並行処理とライフサイクル

- 長時間動く component は `Run(context.Context) error` と `Shutdown(context.Context) error` のペアで扱う。
- shutdown は idempotent にする。複数回呼ばれても panic や二重 close を起こさない。
- channel の close ownership を明確にする。送信側が閉じるか、shutdown 専用 channel を使う。
- subscriber channel は buffered channel を使い、遅い購読者で全体を止めない。
- router status のような状態は mutex で保護し、購読受付可否と起動可否を明示的に判断する。

## 設定と observability

設定は environment variables から読み込みます。`Config` struct は `mapstructure` tag で環境変数名を明示し、`Load()` 内で trim、normalize、validation を完了させます。

logger は `slog` の JSON handler を標準とし、trace は OTLP endpoint が設定された場合だけ exporter を有効にします。HTTP client/server は `otelhttp` で計装し、service name は `SERVICE_NAME` から決めます。

