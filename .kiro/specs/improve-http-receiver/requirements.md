# Requirements Document

## Introduction
HTTP receiver は現在、時間付き launch request を受理する操作だけを提供している。この spec では scope を小さく保ち、既存 request 受信時の ScaledObject 存在確認、ScaledObject 一覧取得、request 削除の 3 機能だけを追加する。

ここでの ScaledObject の存在は、この service が KEDA external scaler として扱っている対象を指す。クラスタ上のリソースへ直接問い合わせて実在性を確認することは含めない。

## Boundary Context
- **In scope**: 既存 `POST /requests` の ScaledObject 存在確認、ScaledObject 一覧取得、request 削除。
- **Out of scope**: request 一覧取得、request 個別取得、認証・認可、audit log、request 永続化、service restart 後の request 復元、クラスタ API への直接問い合わせによる ScaledObject 実在確認。
- **Adjacent expectations**: 既存の KEDA external scaler gRPC behavior は維持し、request 削除の結果は KEDA から観測される active state に反映される。

## Requirements

### Requirement 1: ScaledObject 存在確認付き request 受理
**Objective:** As an 外部クライアント利用者, I want 存在する ScaledObject に対してだけ launch request を受理してほしい, so that 誤った対象への request が成功したように見えない。

#### Acceptance Criteria
1. When 外部クライアントが存在する ScaledObject を指定して launch request を送信する, the HTTP receiver shall request window を受理し、実際に使用される開始時刻と終了時刻を返す。
2. If 外部クライアントが存在しない ScaledObject を指定して launch request を送信する, the HTTP receiver shall request を拒否し、4xx error response を返す。
3. If launch request が ScaledObject 不存在によって拒否される, the HTTP receiver shall その request window を active state 判定に反映しない。
4. When 外部クライアントが同じ ScaledObject と request ID で launch request を再送信する, the HTTP receiver shall 既存 request window を更新対象として扱う。

### Requirement 2: ScaledObject 一覧取得
**Objective:** As an 運用者, I want HTTP receiver が扱っている ScaledObject の一覧を取得したい, so that request 操作の対象を事前に確認できる。

#### Acceptance Criteria
1. When 外部クライアントが ScaledObject 一覧を要求する, the HTTP receiver shall 現在 request 操作の対象として存在する ScaledObject 一覧を返す。
2. When 外部クライアントが ScaledObject 一覧を要求し、対象 ScaledObject が存在しない, the HTTP receiver shall 空の一覧を成功 response として返す。
3. The HTTP receiver shall 一覧内の各 ScaledObject について namespace と name を返す。
4. The HTTP receiver shall ScaledObject 一覧取得によって request window の作成、更新、削除、active state を変更しない。

### Requirement 3: request 削除
**Objective:** As an 運用者, I want 不要になった request window を削除したい, so that ScaledObject の active state を意図した状態へ戻せる。

#### Acceptance Criteria
1. When 外部クライアントが存在する ScaledObject と既存 request ID を指定して request 削除を要求する, the HTTP receiver shall 該当 request window を削除したことを成功 response で示す。
2. When request window が削除される, the HTTP receiver shall 同じ request ID の再削除を成功扱いにしない。
3. When active な request window が削除され、同じ ScaledObject に他の active request window がない, the HTTP receiver shall KEDA から次に観測される active state に inactive を反映できる状態にする。
4. If 外部クライアントが存在する ScaledObject と存在しない request ID を指定して request 削除を要求する, the HTTP receiver shall 4xx error response を返す。
5. If 外部クライアントが存在しない ScaledObject を指定して request 削除を要求する, the HTTP receiver shall 4xx error response を返す。

### Requirement 4: error response と client-visible contract
**Objective:** As an 外部クライアント利用者, I want request 操作の成功と失敗を一貫した形で扱いたい, so that 呼び出し側が原因に応じた制御を実装できる。

#### Acceptance Criteria
1. If request 操作が利用者入力の問題で失敗する, the HTTP receiver shall `message` を含む error response を返す。
2. If request 操作が失敗する, the HTTP receiver shall secret、内部 address、stack trace を response body に含めない。
3. When 外部クライアントが project-provided client を使って ScaledObject 一覧取得または request 削除を呼び出す, the project-provided client shall 成功 response を domain value として返す。
4. If project-provided client が request 操作の 4xx error response を受け取る, the project-provided client shall 呼び出し側が status code と message を判別できる error を返す。

