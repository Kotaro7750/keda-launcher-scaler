# Implementation Plan

- [ ] 1. HTTP 契約と生成コードの土台を整える
- [x] 1.1 HTTP 契約に ScaledObject 一覧、request 削除、not found error を追加する
  - ScaledObject 一覧は namespace/name の配列を返し、対象が存在しない場合も空配列の成功 response になるようにする。
  - request 削除は対象 ScaledObject、request ID、削除された有効 window を返し、unknown ScaledObject と unknown request ID を not found error として表現する。
  - launch request は unknown ScaledObject を成功扱いにしない error response を契約上で表現する。
  - 完了時にはすべての error response が `message` のみを持つ body として表現され、success response と混ざらない。
  - _Requirements: 1.2, 2.1, 2.2, 2.3, 3.1, 3.4, 3.5, 4.1, 4.2_
  - _Boundary: OpenAPIContract_

- [x] 1.2 更新した HTTP 契約から server/client の生成面を更新する
  - 生成された server 側に追加 endpoint の handler 接続点が現れるようにする。
  - 生成された client 側に ScaledObject 一覧と request 削除の呼び出し面が現れるようにする。
  - 完了時には生成コードが手編集ではなく contract から再生成され、後続実装が追加 operation を参照できる。
  - _Requirements: 2.3, 3.1, 4.3, 4.4_
  - _Boundary: OpenAPIContract_

- [ ] 2. Server domain の ScaledObject registry と request state を整える
- [x] 2.1 router が観測済み ScaledObject を request 操作対象として扱えるようにする
  - KEDA から観測された ScaledObject key を registry として保持し、一覧取得と存在確認はその snapshot だけを参照する。
  - 一覧取得と存在確認は request window、subscriber、active state を変更しない読み取り操作にする。
  - unknown ScaledObject に対する request 操作では、新しい ScaledObject entry を暗黙作成しない。
  - 完了時には registry が空なら空一覧になり、known key は namespace/name として返せる。
  - _Requirements: 1.1, 1.2, 1.3, 2.1, 2.2, 2.3, 2.4, 3.5_
  - _Boundary: ArbitratorRouter_

- [x] 2.2 (P) arbitrator が request window を削除して active state を再計算できるようにする
  - 既存 request ID の削除は削除対象の window を返し、request state から除外する。
  - 同じ request ID の再削除は成功扱いにせず not found として扱う。
  - active window の削除後、他に active window がなければ次の観測で inactive になる状態を通知できるようにする。
  - 完了時には削除された window が active 判定に含まれず、subscriber が再計算後の状態を受け取れる。
  - _Requirements: 3.1, 3.2, 3.3_
  - _Boundary: Arbitrator_

- [x] 2.3 router が request 送信と削除を known ScaledObject に限定して仲介する
  - launch request は known ScaledObject のみ per-ScaledObject state へ渡し、same ScaledObject plus same request ID は既存 window の更新として扱う。
  - request 削除は known ScaledObject の既存 request ID だけを削除し、unknown ScaledObject と unknown request ID を区別して not found にする。
  - 完了時には rejected request が active state に反映されず、delete 後の state が router 経由で KEDA から観測できる。
  - _Depends: 2.1, 2.2_
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 3.1, 3.2, 3.3, 3.4, 3.5_
  - _Boundary: ArbitratorRouter_

- [ ] 3. HTTP receiver の request 操作を domain state に接続する
- [x] 3.1 launch request を ScaledObject 存在確認付きで受理する
  - request window の domain validation と時刻正規化は既存の責務を維持し、その後に ScaledObject existence を確認する。
  - known ScaledObject では request window を受理し、実際に使われる開始時刻と終了時刻を response に返す。
  - unknown ScaledObject では 4xx error response を返し、request window を active state 判定へ渡さない。
  - 完了時には同じ ScaledObject と request ID の再送信が既存 window の更新として扱われる。
  - _Depends: 2.3_
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 4.1, 4.2_
  - _Boundary: HTTPReceiverServer, ArbitratorRouter_

- [x] 3.2 ScaledObject 一覧 endpoint を router snapshot として返す
  - 一覧 endpoint は現在 request 操作の対象として存在する ScaledObject の namespace/name を返す。
  - 対象がない場合は空一覧の成功 response にする。
  - 一覧取得では request window の作成、更新、削除、active state 変更を行わない。
  - 完了時には HTTP response の一覧が router snapshot と一致し、副作用なしで繰り返し呼び出せる。
  - _Depends: 2.1_
  - _Requirements: 2.1, 2.2, 2.3, 2.4, 4.1, 4.2_
  - _Boundary: HTTPReceiverServer, ArbitratorRouter_

- [x] 3.3 request 削除 endpoint を router の削除結果に対応させる
  - known ScaledObject と既存 request ID の削除では、削除された request window を成功 response として返す。
  - unknown request ID と unknown ScaledObject は 4xx error response に変換し、secret、内部 address、stack trace を body に含めない。
  - 完了時には active request を削除すると、同じ ScaledObject に他の active request がない場合に KEDA の次回観測で inactive が見える。
  - _Depends: 2.3_
  - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 4.1, 4.2_
  - _Boundary: HTTPReceiverServer, ArbitratorRouter_

- [x] 4. project-provided client を追加操作に対応させる
- [x] 4.1 public client API に ScaledObject 一覧と request 削除の domain value を追加する
  - 外部利用者が generated type を意識せず、ScaledObject と削除結果を domain value として扱えるようにする。
  - client interface は launch に加えて一覧取得と削除を表現する。
  - 完了時には HTTP 固有の generated type や status 表現が public client API に漏れない。
  - _Requirements: 2.3, 3.1, 4.3, 4.4_
  - _Boundary: ClientDomainAPI_

- [x] 4.2 HTTP client wrapper が追加操作の成功 response と API error を変換する
  - ScaledObject 一覧と request 削除の success response を public client の domain value に変換する。
  - path parameter と request ID は利用者入力として正規化してから HTTP 呼び出しに渡す。
  - 4xx error response は呼び出し側が status code と message を判別できる error に変換する。
  - 完了時には `message` body がある error ではその message が優先され、未知の body でも内部詳細を増やさずに判別可能な error になる。
  - _Depends: 1.2, 4.1_
  - _Requirements: 2.3, 3.1, 3.4, 3.5, 4.3, 4.4_
  - _Boundary: ClientWrapper_

- [x] 5. 統合 wiring と repo-owned behavior を検証する
- [x] 5.1 KEDA service が valid ScaledObjectRef を router registry に反映する統合を行う
  - valid ScaledObjectRef を扱う既存 gRPC operation は、active state の問い合わせや stream 開始前に対象 ScaledObject を request 操作可能な key として登録する。
  - invalid ScaledObjectRef に対する既存の gRPC error behavior と metric response shape は変えない。
  - 完了時には KEDA が一度 valid key を観測した後、HTTP 側の ScaledObject 一覧と launch/delete 対象に同じ key が現れる。
  - _Depends: 2.1_
  - _Requirements: 1.1, 1.2, 1.3, 2.1, 2.2, 2.3, 2.4_
  - _Boundary: KEDAService, ArbitratorRouter_

- [x] 5.2 server bootstrap で HTTP receiver と router 管理操作を接続する
  - HTTP receiver が request channel だけでなく ScaledObject lookup/list/delete の domain operation を呼べるように起動時の組み立てを更新する。
  - 既存の HTTP listen、gRPC listen、graceful shutdown、JSON logging の構成は維持する。
  - 完了時には server binary の起動構成で launch/list/delete が同じ router state を共有する。
  - _Depends: 2.3, 3.1, 3.2, 3.3, 5.1_
  - _Requirements: 1.1, 1.2, 1.3, 2.1, 2.4, 3.1, 3.3, 3.5_
  - _Boundary: HTTPReceiverServer, ArbitratorRouter, ServerBootstrap_

- [x] 5.3 server domain と KEDA integration の behavior tests を追加・更新する
  - request 削除、再削除、delete 後の inactive 反映を repo-owned behavior として確認する。
  - unknown ScaledObject の launch/delete が arbitrator を暗黙作成せず、一覧取得が read-only snapshot であることを確認する。
  - valid ScaledObjectRef 処理後に HTTP request 操作対象として同じ key が登録されることを確認する。
  - 完了時には arbitrator、router、KEDA service の tests が外部ライブラリ内部ではなく domain behavior を固定している。
  - _Depends: 2.1, 2.2, 2.3, 5.1_
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 2.1, 2.2, 2.3, 2.4, 3.1, 3.2, 3.3, 3.4, 3.5_
  - _Boundary: Arbitrator, ArbitratorRouter, KEDAService_

- [x] 5.4 HTTP receiver と client wrapper の behavior tests を追加・更新する
  - launch は known ScaledObject だけを受理し、unknown ScaledObject では error response と state 非変更を確認する。
  - ScaledObject 一覧は namespace/name と空一覧を返し、副作用がないことを確認する。
  - request 削除は success、unknown request、unknown ScaledObject を status code と message で判別できることを確認する。
  - client wrapper は一覧と削除の success response を domain value に変換し、4xx response を status code と message を持つ error に変換する。
  - 完了時には HTTP contract 境界と public client 境界の repo-owned behavior が tests で確認されている。
  - _Depends: 3.1, 3.2, 3.3, 4.2_
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 2.1, 2.2, 2.3, 2.4, 3.1, 3.2, 3.3, 3.4, 3.5, 4.1, 4.2, 4.3, 4.4_
  - _Boundary: HTTPReceiverServer, ClientWrapper_

- [x] 5.5 生成物と全体テストで feature 完了条件を確認する
  - HTTP contract からの生成結果に未反映の差分が残っていないことを確認する。
  - 全 package の tests を実行し、receiver、arbitrator、scaler、client wrapper の behavior が通ることを確認する。
  - 完了時には生成コード、server/client build、repo-owned tests が同じ HTTP contract と domain behavior に整合している。
  - _Depends: 5.2, 5.3, 5.4_
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 2.1, 2.2, 2.3, 2.4, 3.1, 3.2, 3.3, 3.4, 3.5, 4.1, 4.2, 4.3, 4.4_
  - _Boundary: OpenAPIContract, HTTPReceiverServer, ArbitratorRouter, Arbitrator, KEDAService, ClientWrapper_

## Implementation Notes

- `oapi-codegen` v2.6.0 の生成物は `github.com/oapi-codegen/runtime` v1.4.0 を前提にしていたため、生成後に runtime 依存を更新した。
- `ArbitratorRouter` の ScaledObject 一覧と存在確認は、追加 map を持たず既存 `arbitrators` map の read-only snapshot で実装した。
- `Arbitrator.delete` は request map から window を除去し、再削除を not found にし、削除後の active 再計算を changed signal で起こす。
- launch request は HTTP receiver が `HasScaledObject` 確認後に既存 `requestCh` へ流し、router の管理 API は `HasScaledObject` / `ListScaledObjects` / `DeleteRequest` に絞った。
- HTTP receiver は requestManager を受け取ると launch 前に ScaledObject 存在確認を行い、`cmd/keda-launcher-scaler` も router を直接渡すようにした。
- `GET /scaledobjects` は requestManager の snapshot を `ScaledObjectList` に変換し、返却順を namespace/name で安定化した。
- `DELETE /scaledobjects/{namespace}/{name}/requests/{requestId}` は requestManager の not-found sentinel を 404 に変換し、成功時は削除された window をそのまま返す。
