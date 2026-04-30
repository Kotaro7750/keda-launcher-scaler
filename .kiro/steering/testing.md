# Testing Standards

このプロジェクトのテストは、外部ライブラリの挙動ではなく、このリポジトリが保証する domain behavior と lifecycle behavior を確認します。

## Philosophy

- test は実装詳細ではなく振る舞いを固定する。
- Go 標準の `testing` を基本にし、必要以上の test framework を増やさない。
- 外部 protocol や生成コードそのものの再検証は避け、repo-owned な変換、validation、lifecycle をテストする。
- 時刻や context cancellation を含むテストは timeout を置き、hang を失敗として扱う。

## Organization

テストは対象 package に co-locate し、`*_test.go` として置きます。

```text
internal/server/receiver/request.go
internal/server/receiver/request_test.go
```

package-private の状態を検証する必要がある lifecycle/concurrency テストは同一 package で書きます。公開 API だけで十分な package は外部 package test にしてもよいですが、この repo では既存の同一 package style に合わせます。

## Test Targets

優先してテストするもの:

- request normalization と domain validation
- HTTP request が arbitrator の `RequestWindow` に変換されること
- generated HTTP client wrapper が domain type と API error を変換すること
- arbitrator/router の active 判定、購読、shutdown
- config loader の必須値、duration、URL、boolean などの validation
- gRPC server の context cancellation / graceful shutdown

避けるもの:

- `oapi-codegen` が schema 通りの型を生成すること自体の確認
- Echo、Viper、gRPC など外部ライブラリ内部の挙動確認
- log message の文言だけを固定する brittle なテスト

## Table Driven Tests

validation や変換は table driven test を基本にします。

```go
tests := []struct {
	name    string
	input   RequestInput
	want    arbitrator.RequestWindow
	wantErr string
}{...}
```

error は必要な contract だけを比較します。完全一致が repo-owned な message contract の場合は許容し、wrap される可能性がある設定エラーなどは `strings.Contains` で意図を確認します。

## Time and Concurrency

- deterministic にしたい時刻は `time.Parse(time.RFC3339, ...)` や固定 `time.Date` を使う。
- 現在時刻に依存する lifecycle テストは `time.Now()` を基準に短い window を作る。
- goroutine の終了待ちは `doneCh` と `time.After` を併用する。
- router の状態待ちは helper に閉じ込め、失敗時は `t.Fatal` で明確にする。

```go
select {
case err := <-doneCh:
	// assert
case <-time.After(time.Second):
	t.Fatal("Run did not return after context cancellation")
}
```

## Test Helpers

helper はテストの意図を隠さない範囲で作ります。

- `mustParseTime(t, value)` のような入力準備 helper は許可する。
- `waitForRouterRunning` のような非同期待ち helper は timeout を内包する。
- 大きな fixture builder より、最小限の struct literal を優先する。
- helper は `t.Helper()` を呼ぶ。

## Commands

通常の検証は次を使います。

```sh
go test ./...
```

並行処理や shutdown を触った場合は race detector も検討します。

```sh
go test -race ./...
```

OpenAPI YAML を変更した場合は生成コードを更新し、テスト前に差分を確認します。

```sh
go generate ./...
go test ./...
```

