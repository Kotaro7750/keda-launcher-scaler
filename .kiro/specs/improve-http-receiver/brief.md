# Brief: improve-http-receiver

## Problem
HTTP receiver は現在 `POST /requests` による launch request の受理だけを提供している。受理時に ScaledObject の存在確認を行わないため、KEDA から参照されていない `scaledObject.namespace/name` に対する request も受理されたように見える。

また、運用者や外部クライアントは、この service が現在扱っている ScaledObject を HTTP API から確認できない。不要になった request window を削除する操作もなく、active state を明示的に戻す手段がない。

## Current State
HTTP API contract の source of truth は `internal/common/contracts/receivers/http/openapi.yaml` で、server/client の生成コードは同じ YAML から `oapi-codegen` で生成する。

現在の API は `POST /requests` のみで、受理した `RequestWindow` を channel 経由で `arbitrator.Router` へ渡す。router は `ScaledObjectKey` ごとに `arbitrators` map で arbitrator を保持し、KEDA external scaler gRPC service は router の `Subscribe` / `IsActive` を使って active state を扱う。

## Desired Outcome
HTTP receiver が以下の 3 つだけを提供する。

- 既存 `POST /requests` で、対象 ScaledObject が存在しない場合に request を拒否する。
- この service が扱っている ScaledObject 一覧を返す。
- ScaledObject と request ID を指定して request を削除する。

OpenAPI YAML、server handler、必要な domain 操作、公開 HTTP client wrapper、repo-owned behavior tests が一貫して更新されている。

## Approach
推奨アプローチは「OpenAPI-first の小さな HTTP receiver 拡張 + 既存 `arbitrators` map による存在確認」とする。

OpenAPI YAML に ScaledObject 一覧 endpoint と request 削除 endpoint を追加し、`go generate ./...` で server/client の生成コードを更新する。`POST /requests` は既存 shape を維持し、受理前に router の `arbitrators` map で対象 ScaledObject を lookup する。

ScaledObject の存在確認は Kubernetes API へ問い合わせない。KEDA external scaler から valid `ScaledObjectRef` を受けたときに router が作成した `arbitrators` map の key set を、この feature における ScaledObject 一覧と存在判定の source とする。

## Scope
- **In**: `POST /requests` における ScaledObject 存在確認。
- **In**: ScaledObject 一覧取得 endpoint。
- **In**: request 削除 endpoint。
- **In**: delete 後の active state 再計算と subscriber notification。
- **In**: `pkg/client` と `pkg/client/http` の ScaledObject 一覧・request 削除 API 拡張。
- **In**: OpenAPI 生成コード更新と repo-owned behavior tests。
- **Out**: request 一覧取得。
- **Out**: request 個別取得。
- **Out**: Kubernetes API server へ問い合わせる ScaledObject 実在確認。
- **Out**: 認証・認可、multi-tenant access control、audit log。
- **Out**: request 永続化、restart 後の request 復元。
- **Out**: KEDA external scaler gRPC API contract の変更。

## Boundary Candidates
- HTTP contract boundary: `internal/common/contracts/receivers/http/openapi.yaml` を正として endpoint、schema、status code を定義する。
- HTTP receiver boundary: HTTP request/response と domain 操作の変換、OpenAPI validation、HTTP error mapping を担当する。
- Router/arbitrator boundary: `arbitrators` map key set による ScaledObject lookup と request delete を担当する。
- Client boundary: `pkg/client` は transport-neutral API、`pkg/client/http` は generated HTTP client との変換を担当する。

## Out of Boundary
- request list/get API はこの spec では扱わない。
- Kubernetes API client の導入や RBAC 設計はこの spec では扱わない。
- ScaledObject の存在を cluster 上の実リソースとして検証する設計にはしない。
- HTTP receiver に readiness probe や health endpoint を追加しない。
- expired request の pruning 方針を永続化や履歴管理へ拡張しない。

## Upstream / Downstream
- **Upstream**: 既存の `POST /requests` contract、`receiver.NormalizeRequest`、`arbitrator.Router`、ScaledObject ごとの `arbitrator` 内部 request map、KEDA external scaler からの `Subscribe` / `IsActive` 呼び出し。
- **Downstream**: CLI や Slack app などの外部 client が対象 ScaledObject の確認や request 削除操作を実装できる。

## Existing Spec Touchpoints
- **Extends**: 既存 spec はまだないため対象なし。
- **Adjacent**: HTTP API standards、Go architecture standards、testing standards、error-handling standards。特に OpenAPI-first、server/client generated code、repo-owned behavior tests の方針に従う。

## Constraints
Markdown と spec 文書は日本語で書く。

HTTP contract 変更は必ず `internal/common/contracts/receivers/http/openapi.yaml` から開始し、生成コードを手で編集しない。生成後は `go generate ./...` と `go test ./...` を基本検証とする。

HTTP request operation は未知 ScaledObject に対して `arbitrators` map を増やさない。KEDA service 側だけが valid ScaledObjectRef に対応する arbitrator を作成する。

HTTP error body は既存 contract の `{ "message": "..." }` を維持する。利用者入力の問題や存在しない ScaledObject/request は 4xx として返し、内部 error や秘密情報を response に含めない。

