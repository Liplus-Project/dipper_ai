# Architecture

## 設計判断の記録（ADR）

### ADR-001: 常駐デーモン方式の採用（v1.5.0〜）

**決定**: systemd oneshot+timer 方式から常駐デーモン方式へ変更する。

**背景**:
DDNS クライアントには 2 つの独立した時間軸がある。

| 設定 | 役割 | 典型的な間隔 |
|------|------|------------|
| `DDNS_TIME` | IP 変化検知のチェック間隔 | 1〜60 分 |
| `UPDATE_TIME` | keepalive 送信間隔（サービスの失効防止） | 1 日〜30 日 |

単一の systemd タイマーではこの 2 軸を独立管理できず、
`DDNS_TIME > UPDATE_TIME` の組み合わせが正しく動作しない仕様バグがあった。

**決定理由**:
1. 2 本の goroutine ticker がプロセス内部で 2 つの間隔を完全に独立管理できる
2. 単一プロセス = 単一ログストリーム → `journalctl -u dipper_ai` だけでデバッグ完結
3. `install.sh` からタイマー動的生成ロジックが不要になりシンプル化できる
4. プロセスクラッシュ時は systemd の `Restart=on-failure` が自動復帰を担う

**受容したデメリット**:
- 常駐プロセスになるためメモリを常時使用（DDNS クライアントの規模では無視できる）
- oneshot より goroutine / シグナル管理が必要になる

---

## パッケージ構成

```
dipper_ai/
├── cmd/dipper_ai/        # エントリポイント (main.go)
├── internal/
│   ├── config/           # 設定ファイルのパース・バリデーション
│   ├── ddns/             # DDNS プロバイダ実装（MyDNS / Cloudflare）
│   ├── ip/               # グローバル IP アドレス取得
│   ├── lock/             # 多重起動防止ロック
│   ├── mode/             # コマンドロジック（daemon / update / check / keepalive / err_mail）
│   ├── state/            # 状態ファイル読み書き
│   └── timegate/         # タイムゲート（IP_CACHE_TIME / ERR_CHK_TIME 制御に使用）
├── docs/                 # このドキュメント群
├── scripts/              # install.sh / uninstall.sh
└── systemd/              # dipper_ai.service
```

---

## 各パッケージの役割

### `config`

`user.conf` を読み込み、`Config` 構造体に変換します。

- shell-style `key=value` 形式、インラインコメント・クォート除去対応
- `boolVal`: `on` / `off` / `1` / `0` / `true` / `false`（大小文字不問）
- `intMin`: 最小値を強制（`UPDATE_TIME` に使用）
- `intGate`: `0` = 無効化、それ以外は最小値を強制（`IP_CACHE_TIME`、`ERR_CHK_TIME` に使用）
- 時間指定は `5m`、`2h`、`1d`、`30s`、または整数（分）で記述可能
- MyDNS エントリは `MYDNS_N_ID` が存在する限りインデックスをインクリメントして解析
- Cloudflare エントリは `CF_N_ENABLED` が存在する限り同様にインクリメント

```go
type Config struct {
    StateDir     string
    IPv4, IPv6   bool
    IPv4DDNS     bool
    IPv6DDNS     bool
    DDNSTime     int  // minutes; daemon check interval (0 = default 5min)
    UpdateTime   int  // minutes; keepalive interval (0 = disabled)
    IPCacheTime  int  // minutes; 0 = disabled
    ErrChkTime   int  // minutes; 0 = disabled
    MyDNS        []MyDNSEntry
    Cloudflare   []CloudflareEntry
    // ...email fields
}
```

### `ip`

`ip.Fetch(ipv4, ipv6 bool)` でグローバル IP アドレスを取得します。
`dig` 等の外部コマンドに依存せず、外部 HTTP API を使用します。

テスト時は `DIPPER_AI_FAKE_IP_V4` / `DIPPER_AI_FAKE_IP_V6` 環境変数で差し替え可能です。

### `ddns`

DDNS プロバイダごとの HTTP 更新実装。

**MyDNS** (`mydns.go`):
- `UpdateMyDNSIPv4(entry, url) ProviderResult`
- `UpdateMyDNSIPv6(entry, url) ProviderResult`
- HTTP GET + Basic Auth（keepalive も同一エンドポイント）

**Cloudflare** (`cloudflare.go`):
- `UpdateCloudflare(entry, ip, recordType, zonesURL) ProviderResult`
- ゾーン名 → ゾーン ID 解決 → レコード検索 → PATCH 更新
- `Authorization: Bearer <token>` ヘッダ
- keepalive 不要（API 登録は失効しない）

### `timegate`

`gate_<name>` ファイルに RFC3339 タイムスタンプを書き込み、前回実行からの経過時間を判定します。
現在は `IP_CACHE_TIME`（IP キャッシュ有効期間）と `ERR_CHK_TIME`（エラーメール間隔）にのみ使用。
チェック間隔・keepalive 間隔はデーモンの ticker が担うため、`DDNS_TIME` / `UPDATE_TIME` にはゲートファイルを使用しません。

### `state`

IP アドレスや DDNS 結果・エラーログを `STATE_DIR` のファイルに保存・読み込みします。

| ファイル名 | 内容 |
|------------|------|
| `cache_<entryKey>_<family>` | ドメインごとの最終送信 IP（例: `cache_mydns_0_ipv4`） |
| `ddns_errors` | DDNS 更新エラーのログ（`err_mail` が読んでクリア） |
| `ddns_result_<entryKey>` | 最終 DDNS 更新結果（`ok` / `fail:...`） |
| `gate_ip_cache` | IP キャッシュのタイムスタンプ |
| `gate_errchk` | エラーメール送信のタイムスタンプ |
| `lock_<command>` | 多重起動防止ロックファイル |

### `lock`

`lock_<command>` ファイルによる排他ロック。同一コマンドの二重起動を防ぎます。

### `mode`

コマンドごとのロジックを実装します。テスト容易性のため、外部依存はパッケージ変数として注入可能にしています。

```go
var (
    ipFetch          = ip.Fetch
    mydnsUpdateIPv4  = ddns.UpdateMyDNSIPv4
    mydnsUpdateIPv6  = ddns.UpdateMyDNSIPv6
    cloudflareUpdate = ddns.UpdateCloudflare
    sendMailFn       = mail.Send
)
```

---

## デーモンの動作フロー

```
systemd が dipper_ai daemon を起動
  │
  ├─ 設定読み込み (config.Load)
  ├─ 起動ログ出力: "starting (check=5m, keepalive=1d)"
  ├─ 10 秒待機（ネットワーク確立猶予）
  ├─ 初回サイクル実行（起動直後に 1 回）
  │
  ├─ checkTicker (DDNS_TIME 間隔)
  │    └─ runCycle()
  │         ├─ mode.Update(cfg)    IP 変化時に DDNS 更新
  │         ├─ mode.Check(cfg)     DNS 解決と実際の IP の一致確認
  │         └─ mode.ErrMail(cfg)   エラー集計・通知
  │
  ├─ keepaliveTicker (UPDATE_TIME 間隔、0 なら無効)
  │    └─ mode.Keepalive(cfg)      全 MyDNS エントリを強制更新
  │
  └─ SIGTERM / SIGINT 受信 → クリーンシャットダウン
```

### keepalive について

DDNS サービスは定期的な更新がないと登録を失効させる場合がある（サービスによって期間は異なる）。
`Keepalive` は IP が変化していなくても全 MyDNS エントリへ強制送信することで失効を防ぐ。
Cloudflare は API 登録が失効しないため keepalive は不要であり、`Keepalive` では対象外となる。

---

## update / check / keepalive の責務分担

| コマンド | トリガー | 動作 |
|----------|----------|------|
| `update` | checkTicker | キャッシュと現在 IP を比較し、差分があるエントリのみ更新 |
| `check` | checkTicker（updateの直後） | DNS 解決結果と現在 IP を比較し、不一致なら update キャッシュをリセット |
| `keepalive` | keepaliveTicker | 全 MyDNS エントリを無条件で送信（Cloudflare はスキップ） |
| `err_mail` | checkTicker（checkの直後） | エラーログを確認し、閾値超過でメール通知 |

`check` が IP とキャッシュの不一致を検知した場合、対象エントリのキャッシュをリセットすることで次の `update` が強制的に再送信を行う。

---

## テスト構造

| パッケージ | テスト手法 |
|------------|-----------|
| `config` | ユニットテスト（`ParseFile` に一時ファイルを渡す） |
| `ddns` | `httptest.NewServer` による HTTP モックサーバ |
| `mode` | パッケージ変数差し替えによる関数インジェクション |
| `acceptance` | バイナリをビルドして `exec.Command` で呼び出すブラックボックステスト |

受け入れテストは外部 API に依存しません。
`DIPPER_AI_FAKE_IP_V4` / `DIPPER_AI_FAKE_IP_V6` / `DIPPER_AI_FAKE_DNS` 環境変数で外部依存を差し替えます。

---

## 状態ファイルの配置例

```
/etc/dipper_ai/state/
├── cache_mydns_0_ipv4       # 203.0.113.42
├── cache_mydns_0_ipv6       # 2001:db8::1
├── cache_cf_0_A             # 203.0.113.42
├── ddns_errors              # エラーログ（err_mail が読んでクリア）
├── gate_ip_cache            # 2025-01-15T10:00:00+09:00
├── gate_errchk              # 2025-01-15T09:00:00+09:00
└── lock_daemon              # 多重起動防止
```
