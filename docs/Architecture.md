# Architecture

## パッケージ構成

```
dipper_ai/
├── cmd/dipper_ai/        # エントリポイント (main.go)
├── internal/
│   ├── config/           # 設定ファイルのパース・バリデーション
│   ├── ddns/             # DDNS プロバイダ実装
│   ├── ip/               # グローバル IP アドレス取得
│   ├── lock/             # 多重起動防止ロック
│   ├── mode/             # コマンドロジック (update / check / err_mail)
│   ├── state/            # 状態ファイル読み書き
│   └── timegate/         # タイムゲート（実行間隔制御）
├── scripts/              # install.sh / uninstall.sh
└── systemd/              # .service / .timer unit ファイル
```

---

## 各パッケージの役割

### `config`

`user.conf` を読み込み、`Config` 構造体に変換します。

- shell-style `key=value` 形式、インラインコメント・クォート除去対応
- `boolVal`: `on` / `off` / `1` / `0` / `true` / `false`（大小文字不問）
- `intMin`: 最小値を強制（`UPDATE_TIME`、`DDNS_TIME` に使用）
- `intGate`: `0` = 無効化、それ以外は最小値を強制（`IP_CACHE_TIME`、`ERR_CHK_TIME` に使用）
- MyDNS エントリは `MYDNS_0_ID` が存在する限りインデックスをインクリメントして解析
- Cloudflare エントリは `CF_0_ENABLED` が存在する限り同様にインクリメント

```go
type Config struct {
    StateDir     string
    IPv4, IPv6   bool
    UpdateTime   int  // minutes
    IPCacheTime  int  // 0 = disabled
    MyDNS        []MyDNSEntry
    Cloudflare   []CloudflareEntry
    // ...
}
```

### `ip`

`ip.Fetch(ipv4, ipv6 bool)` でグローバル IP アドレスを取得します。
外部 HTTP API（`ipv4.icanhazip.com` / `ipv6.icanhazip.com` 等）を使用し、`dig` などの外部コマンドに依存しません。

### `ddns`

DDNS プロバイダごとの HTTP 更新実装。

**MyDNS** (`mydns.go`):
- `UpdateMyDNSIPv4(entry, url) ProviderResult`
- `UpdateMyDNSIPv6(entry, url) ProviderResult`
- HTTP GET + Basic Auth ヘッダ

**Cloudflare** (`cloudflare.go`):
- `UpdateCloudflare(entry, ip, recordType, zonesURL) ProviderResult`
- 3 ステップ: ゾーン名 → ゾーン ID 解決 → レコード検索 → PATCH 更新
- `Authorization: Bearer <token>` ヘッダ

`ProviderResult` は成功・失敗とエラーメッセージを保持します。

### `timegate`

`gate_<name>` ファイルに RFC3339 タイムスタンプを書き込み、前回実行からの経過時間を判定します。

```go
gate := timegate.New(stateDir, "update", 1440*time.Minute)
if gate.ShouldRun() {
    // 実行
    gate.Touch()
}
```

### `state`

IP アドレスや DDNS エラーログを `STATE_DIR` のファイルに保存・読み込みします。

| ファイル名 | 内容 |
|------------|------|
| `ip_ipv4` | 最後に取得した IPv4 アドレス |
| `ip_ipv6` | 最後に取得した IPv6 アドレス |
| `ddns_errors` | DDNS 更新エラーのログ |
| `gate_<name>` | タイムゲートのタイムスタンプ（RFC3339） |
| `lock_<command>` | 多重起動防止ロックファイル |

### `lock`

`lock_<command>` ファイルによる排他ロック。同一コマンドの二重起動を防ぎます。

### `mode`

コマンドごとのロジックを実装します。テスト容易性のため、外部依存（IP 取得・DDNS 更新・メール送信）はパッケージ変数として注入可能にしています。

```go
var (
    ipFetch         = ip.Fetch
    mydnsUpdateIPv4 = ddns.UpdateMyDNSIPv4
    cloudflareUpdate = ddns.UpdateCloudflare
)
```

---

## データフロー（update コマンド）

```
main()
  └─ config.Load()              設定読み込み
  └─ lock.Acquire()             多重起動防止
  └─ mode.Update(cfg)
       └─ timegate "update"     UPDATE_TIME ゲート確認
       └─ ip.Fetch()            グローバル IP 取得
       └─ state.ReadIP()        前回 IP 読み込み
       └─ [IP 変化あり]
            └─ timegate "ddns"  DDNS_TIME ゲート確認
            └─ ddns.UpdateMyDNSIPv4/IPv6()   MyDNS 更新
            └─ ddns.UpdateCloudflare()       CF 更新
            └─ state.WriteIP()  新 IP を保存
            └─ state.AppendError() エラーがあれば記録
```

---

## テスト構造

| パッケージ | テスト手法 |
|------------|-----------|
| `config` | ユニットテスト（`ParseFile` に文字列を渡す） |
| `ddns` | `httptest.NewServer` によるHTTPモックサーバ |
| `mode` | パッケージ変数差し替えによる関数インジェクション |
| `acceptance` | バイナリをビルドして `exec.Command` で呼び出すブラックボックステスト |

受け入れテストは外部 API・`dig` コマンドに依存しません。
IP 取得が必要なテストでは `IP_CACHE_TIME > 0` + `gate_ip_cache` ファイルの事前配置でキャッシュヒットパスを使用します。
