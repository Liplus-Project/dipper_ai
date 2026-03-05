# Configuration

設定ファイルは shell-style の `key=value` 形式です。
デフォルトのパスは `/etc/dipper_ai/user.conf`（`DIPPER_AI_CONFIG` 環境変数で変更可）。

## 書式ルール

```
KEY=value          # 基本形
KEY=on             # 真偽値: on / off / 1 / 0 / true / false（大小文字不問）
KEY=value # memo   # インラインコメント（値に " #" を含めない）
KEY="value"        # 前後のダブルクォートは自動で除去
# コメント行       # 行頭 # は無視
```

### 時間指定フォーマット

時間に関する設定（`DDNS_TIME`、`UPDATE_TIME`、`IP_CACHE_TIME`、`ERR_CHK_TIME`）はサフィックス形式が使用できます。

| 書式 | 意味 | 例 |
|------|------|----|
| `30s` | 秒（分に切り上げ） | `30s` → 1 分 |
| `5m` | 分 | `5m` → 5 分 |
| `2h` | 時間 | `2h` → 120 分 |
| `1d` | 日 | `1d` → 1440 分 |
| `30`（整数） | 分（後方互換） | `30` → 30 分 |

---

## STATE_DIR

| | |
|--|--|
| **キー** | `STATE_DIR` |
| **型** | string (ディレクトリパス) |
| **デフォルト** | `/etc/dipper_ai/state` |

タイムスタンプ・IP キャッシュ・エラーログなどの状態ファイルを保存するディレクトリ。
存在しない場合は実行時に自動生成されます。

```conf
STATE_DIR=/etc/dipper_ai/state
```

---

## IP アドレス設定

| キー | 型 | デフォルト | 説明 |
|------|----|-----------|------|
| `IPV4` | bool | `on` | IPv4 アドレスを取得・追跡する |
| `IPV6` | bool | `off` | IPv6 アドレスを取得・追跡する |
| `IPV4_DDNS` | bool | `on` | IPv4 で DDNS を更新する（A レコード） |
| `IPV6_DDNS` | bool | `on` | IPv6 で DDNS を更新する（AAAA レコード） |

```conf
IPV4=on
IPV6=off
IPV4_DDNS=on
IPV6_DDNS=on
```

---

## タイム設定

### DDNS_TIME — デーモンのチェック間隔

| | |
|--|--|
| **キー** | `DDNS_TIME` |
| **デフォルト** | `5m`（5 分） |
| **最小値** | `1m` |

デーモン（`dipper_ai daemon`）が IP チェック・DDNS 更新サイクルを実行する間隔。
`0` を指定した場合はデフォルト（5 分）が適用されます。

IP 変化への応答速度を上げたい場合は短く、システムリソースを節約したい場合は長く設定します。

```conf
DDNS_TIME=5m    # 5 分ごとにチェック
DDNS_TIME=1m    # 1 分ごと（敏感な運用）
DDNS_TIME=1h    # 1 時間ごと（余裕のある運用）
```

### UPDATE_TIME — keepalive 送信間隔

| | |
|--|--|
| **キー** | `UPDATE_TIME` |
| **デフォルト** | `1d`（1 日） |
| **最小値** | `3m` |
| **無効化** | `0` |

DDNS サービスへの定期 keepalive 送信間隔。
IP が変化していなくても指定間隔で強制更新し、サービスの登録失効を防ぐ。

適切な値はご利用の DDNS サービスの失効期間に合わせて設定してください。
`0` を指定すると keepalive は無効化されます（Cloudflare のみ利用の場合など）。

> **`DDNS_TIME` との独立性**
> `DDNS_TIME` と `UPDATE_TIME` はデーモン内部の独立したタイマーで管理されます。
> `DDNS_TIME=5m`（5 分チェック）と `UPDATE_TIME=30d`（30 日 keepalive）のような
> 任意の組み合わせが正しく動作します。

```conf
UPDATE_TIME=1d     # 1 日ごとに keepalive（MyDNS の推奨）
UPDATE_TIME=7d     # 7 日ごと
UPDATE_TIME=0      # keepalive 無効（Cloudflare のみ利用の場合など）
```

### IP_CACHE_TIME — IP キャッシュ有効期間

| | |
|--|--|
| **キー** | `IP_CACHE_TIME` |
| **デフォルト** | `0`（無効） |
| **最小値** | `15m`（有効時） |
| **無効化** | `0` |

グローバル IP の取得結果をキャッシュする期間。
`0` のとき（デフォルト）は毎回外部 API に問い合わせます。
`DDNS_TIME` が短い場合にキャッシュを設定すると外部 API への負荷を軽減できます。

```conf
IP_CACHE_TIME=0     # キャッシュなし（毎回取得）
IP_CACHE_TIME=15m   # 15 分キャッシュ
```

### ERR_CHK_TIME — エラーメール送信間隔

| | |
|--|--|
| **キー** | `ERR_CHK_TIME` |
| **デフォルト** | `0`（無効） |
| **最小値** | `1m`（有効時） |
| **無効化** | `0` |

エラーログの確認・通知メール送信の最小間隔。
`0` のとき（デフォルト）はチェックのたびに確認します（推奨: `EMAIL_ADR` が設定されている場合は適切な値を設定する）。

```conf
ERR_CHK_TIME=0      # 無効（毎サイクル確認）
ERR_CHK_TIME=1h     # 1 時間ごとに確認
```

---

## MyDNS

[MyDNS.JP](https://www.mydns.jp) への更新設定。エントリは `MYDNS_0_*`、`MYDNS_1_*`、`MYDNS_2_*` ... と増やせます。

### エントリキー（`N` = 0, 1, 2, ...）

| キー | 型 | 必須 | デフォルト | 説明 |
|------|----|------|-----------|------|
| `MYDNS_N_ID` | string | ✓ | — | MyDNS マスター ID（省略するとそのエントリを終端とみなす） |
| `MYDNS_N_PASS` | string | ✓ | — | MyDNS パスワード |
| `MYDNS_N_DOMAIN` | string | | `""` | ドメイン名（ログ識別用） |
| `MYDNS_N_IPV4` | bool | | `on` | このエントリで IPv4 を更新する |
| `MYDNS_N_IPV6` | bool | | `off` | このエントリで IPv6 を更新する |

### URL オーバーライド（省略可）

| キー | デフォルト |
|------|-----------|
| `MYDNS_IPV4_URL` | `https://ipv4.mydns.jp/login.html` |
| `MYDNS_IPV6_URL` | `https://ipv6.mydns.jp/login.html` |

### 例

```conf
MYDNS_0_ID=mydns123456
MYDNS_0_PASS=yourpassword
MYDNS_0_DOMAIN=home.example.com
MYDNS_0_IPV4=on
MYDNS_0_IPV6=off

MYDNS_1_ID=mydns789012
MYDNS_1_PASS=anotherpassword
MYDNS_1_DOMAIN=server.example.com
MYDNS_1_IPV4=on
MYDNS_1_IPV6=on
```

---

## Cloudflare

Cloudflare DNS への更新設定。エントリは `CF_0_*`、`CF_1_*` ... と増やせます。
ゾーン名からゾーン ID を自動解決します。
Cloudflare は API 登録が失効しないため、keepalive（`UPDATE_TIME`）の対象外です。

### エントリキー（`N` = 0, 1, 2, ...）

| キー | 型 | 必須 | デフォルト | 説明 |
|------|----|------|-----------|------|
| `CF_N_ENABLED` | bool | ✓ | — | このエントリを有効化（省略するとそのエントリを終端とみなす） |
| `CF_N_API` | string | 有効時 ✓ | — | Cloudflare API トークン（DNS:Edit 権限） |
| `CF_N_ZONE` | string | 有効時 ✓ | — | ゾーン名（例: `example.com`） |
| `CF_N_DOMAIN` | string | 有効時 ✓ | — | 更新対象 FQDN（例: `home.example.com`） |
| `CF_N_IPV4` | bool | | `on` | A レコードを更新する |
| `CF_N_IPV6` | bool | | `off` | AAAA レコードを更新する |

### URL オーバーライド（省略可）

| キー | デフォルト |
|------|-----------|
| `CLOUDFLARE_URL` | `https://api.cloudflare.com/client/v4/zones` |

### 例

```conf
CF_0_ENABLED=on
CF_0_API=your_api_token_here
CF_0_ZONE=example.com
CF_0_DOMAIN=home.example.com
CF_0_IPV4=on
CF_0_IPV6=off
```

---

## メール通知

エラーが蓄積された場合に `sendmail` 経由で通知します。

| キー | 型 | デフォルト | 説明 |
|------|----|-----------|------|
| `EMAIL_CHK_DDNS` | bool | `off` | IP 変化時に通知（`update` コマンドが送信） |
| `EMAIL_UP_DDNS` | bool | `off` | keepalive 送信時に通知（`keepalive` コマンドが送信） |
| `EMAIL_ADR` | string | `""` | 通知先メールアドレス（`EMAIL_CHK_DDNS` / `EMAIL_UP_DDNS` を使う場合は必須） |

```conf
EMAIL_CHK_DDNS=on
EMAIL_UP_DDNS=off
EMAIL_ADR=admin@example.com
```

> **Note:** メール送信は `sendmail` コマンドに依存します。事前にメール送信環境を構築してください。

---

## 設定例（最小構成・MyDNS のみ）

```conf
# --- IP ---
IPV4=on
IPV6=off

# --- タイム ---
DDNS_TIME=5m        # 5 分ごとに IP チェック
UPDATE_TIME=1d      # 1 日ごとに MyDNS keepalive

# --- MyDNS ---
MYDNS_0_ID=mydns123456
MYDNS_0_PASS=yourpassword
MYDNS_0_DOMAIN=home.example.com
```

## 設定例（MyDNS + Cloudflare 併用）

```conf
# --- IP ---
IPV4=on
IPV6=off

# --- タイム ---
DDNS_TIME=5m
UPDATE_TIME=1d      # MyDNS のみ keepalive、Cloudflare は不要

# --- MyDNS ---
MYDNS_0_ID=mydns123456
MYDNS_0_PASS=yourpassword
MYDNS_0_DOMAIN=home.example.com

# --- Cloudflare ---
CF_0_ENABLED=on
CF_0_API=your_cf_api_token
CF_0_ZONE=example.com
CF_0_DOMAIN=sub.example.com
CF_0_IPV4=on
```
