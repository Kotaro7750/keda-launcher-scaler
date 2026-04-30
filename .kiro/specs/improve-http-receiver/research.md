# Research & Design Decisions

## Summary
- **Feature**: `improve-http-receiver`
- **Discovery Scope**: Extension
- **Key Findings**:
  - HTTP receiver contract は `internal/common/contracts/receivers/http/openapi.yaml` が source of truth であり、server/client の生成コードは同じ YAML から作る必要がある。
  - `router` は既存の `arbitrators map[ScaledObjectKey]*ArbitratorInfo` を持っており、この key set を ScaledObject 一覧と存在確認に使える。
  - request 削除は `arbitrator` 内部の request map を変更し、active state 再計算と subscriber notification につなげる必要がある。

## Research Log

### Existing HTTP contract and code generation
- **Context**: ScaledObject 一覧 endpoint と request 削除 endpoint は外部 contract を変更する。
- **Sources Consulted**:
  - `.kiro/steering/api-standards.md`
  - `internal/common/contracts/receivers/http/openapi.yaml`
  - `internal/server/receiver/http/oapi-codegen.yaml`
  - `pkg/client/http/oapi-codegen.yaml`
- **Findings**:
  - 現在の HTTP API は `POST /requests` のみ。
  - server は `echo-server` と `strict-server` を生成し、client は generated client を `pkg/client/http` で domain type に変換している。
  - error response は `{ "message": "..." }` の単純な shape を維持する方針。
- **Implications**:
  - endpoint と schema は OpenAPI YAML に追加し、生成コードは手編集しない。
  - HTTP client の public API は `pkg/client` の domain type と interface で表現し、generated type を漏らさない。

### Router and arbitrator state
- **Context**: ScaledObject 一覧、存在確認、request 削除は既存 state の読み書きになる。
- **Sources Consulted**:
  - `internal/server/arbitrator/router.go`
  - `internal/server/arbitrator/arbitrator.go`
  - `internal/server/arbitrator/router_test.go`
  - `internal/server/arbitrator/arbitrator_test.go`
- **Findings**:
  - `router` は `arbitrators map[ScaledObjectKey]*ArbitratorInfo` を持ち、ScaledObject key ごとの arbitrator を管理している。
  - `Subscribe` は未知 key に対して arbitrator を作成するため、この map は KEDA から観測済みの ScaledObject registry としても使える。
  - 現状の request routing は未知 ScaledObject に対しても arbitrator を作成できる。
  - `arbitrator` は `map[RequestId]RequestWindow` を持ち、active 判定と pruning を担当している。
- **Implications**:
  - 新しい registry map は追加しない。
  - HTTP request operation は `arbitrators` map の lookup のみを行い、存在しない key に対して arbitrator を作成しない。
  - request delete は request map を削除し、active state の再計算を促す。

### KEDA service integration
- **Context**: Kubernetes API を使わずに ScaledObject 存在を定義する必要がある。
- **Sources Consulted**:
  - `internal/server/scaler/service.go`
  - `.kiro/steering/go-architecture.md`
- **Findings**:
  - gRPC service は `ScaledObjectRef` から `types.ScaledObjectKey` を作り、router の `Subscribe` / `IsActive` を使う。
  - `StreamIsActive` は現在も subscription 作成時に router に key を渡す。
  - `IsActive`、`GetMetrics`、`GetMetricSpec` は現在、未知 key を登録しない。
- **Implications**:
  - KEDA から valid `ScaledObjectRef` を受けた時点で router に ScaledObject key の arbitrator を作成する。
  - HTTP ScaledObject 一覧は `arbitrators` map の key snapshot とする。
  - KEDA gRPC contract 自体は変更しない。

## Architecture Pattern Evaluation

| Option | Description | Strengths | Risks / Limitations | Notes |
|--------|-------------|-----------|---------------------|-------|
| Small OpenAPI-first extension | `POST /requests` existence check、`GET /scaledobjects`、request delete だけを追加する | scope が小さく、既存境界に合う | request list/get は別 spec が必要 | 採用 |
| Full request management API | list/get/delete をまとめて追加する | 管理 API としては完結する | 実装とレビューが大きくなる | 不採用 |
| Kubernetes API lookup | HTTP request ごとに Kubernetes API へ ScaledObject を問い合わせる | cluster 実リソースの存在確認になる | RBAC、client 設定、cache、障害モードが増える | 不採用 |

## Design Decisions

### Decision: ScaledObject existence is the router arbitrator key set
- **Context**: request 操作時に ScaledObject 存在確認が必要だが、Kubernetes API は scope 外。
- **Alternatives Considered**:
  1. Kubernetes API lookup — 実リソースを確認する。
  2. New observed registry map — `arbitrators` とは別に key set を持つ。
  3. Existing `arbitrators` map key set — KEDA service が valid key の arbitrator を作成し、その key set を存在判定に使う。
- **Selected Approach**: router の既存 `arbitrators` map の key set を ScaledObject registry として使う。KEDA service は valid `ScaledObjectRef` を処理するたびに登録 path を通り、HTTP request operation は lookup only にする。
- **Rationale**: 既存 map がすでに ScaledObject ごとの runtime state を表現しており、別 registry map を追加すると二重管理になる。
- **Trade-offs**: service restart 後、KEDA から key が再観測されるまでは HTTP request 操作は unknown ScaledObject として扱われる。
- **Follow-up**: HTTP request operation は unknown key で map を増やさないことを tests で固定する。

### Decision: Request delete is the only request management operation in this spec
- **Context**: request list/get まで含めると scope が大きくなる。
- **Alternatives Considered**:
  1. list/get/delete 全部を実装する。
  2. delete のみに絞り、list/get は別 spec にする。
- **Selected Approach**: この spec では delete のみを持つ。
- **Rationale**: ScaledObject 存在確認と delete 後の active state 反映に集中でき、実装を小さく保てる。
- **Trade-offs**: request 状態確認 API はまだ提供されない。
- **Follow-up**: request list/get が必要になったら別 spec で追加する。

## Synthesis Outcomes
- ScaledObject 一覧と存在確認は、どちらも `arbitrators` map key set の snapshot/lookup として一般化できる。
- request 削除は既存 `arbitrator` state への小さな mutation として扱い、独立 store は作らない。
- 新しい external dependency は不要。既存の Echo、oapi-codegen、router/arbitrator、project-provided client wrapper を拡張する。

## Risks & Mitigations
- `arbitrators` map が HTTP request によって暗黙作成される — HTTP request operation は lookup only とし、unknown ScaledObject tests で map が増えないことを検証する。
- delete 後に active state が古いまま通知される — delete method は state change signal を発行し、router/arbitrator tests で検証する。
- generated code と wrapper のズレ — OpenAPI YAML 更新後に `go generate ./...` と wrapper tests を必須にする。

## References
- `.kiro/steering/api-standards.md` — HTTP contract と OpenAPI-first 方針。
- `.kiro/steering/go-architecture.md` — receiver、arbitrator router、KEDA scaler の責務境界。
- `.kiro/steering/testing.md` — repo-owned behavior を中心にした test 方針。
- `.kiro/steering/error-handling.md` — HTTP error shape と secret 非露出方針。

