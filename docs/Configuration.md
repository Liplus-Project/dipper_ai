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

## タイムゲート（分単位）

実行間隔を制御します。ゲートファイルが `STATE_DIR` に保存されます。

| キー | デフォルト | 最小値 | 説明 |
|------|-----------|--------|------|
| `UPDATE_TIME` | `1440` | 3 | 定期更新の間隔（分） |
| `DDNS_TIME` | `1` | 1 | IP 変化後に DDNS を更新する間隔（分） |
| `IP_CACHE_TIME` | `0` | 15（有効時） | IP キャッシュの有効期間（0 = 無効化） |
| `ERR_CHK_TIME` | `0` | 1（有効時） | エラーメール送信の間隔（0 = 無効化） |

- 設定値が最小値を下回る場合は自動的に最小値に切り上げられます。
- `0` を設定すると「無効化」として扱われる（`IP_CACHE_TIME`、`ERR_CHK_TIME` のみ）。

```conf
UPDATE_TIME=1440    # 1日
DDNS_TIME=1         # 1分
IP_CACHE_TIME=0     # キャッシュなし（毎回取得）
ERR_CHK_TIME=0      # エラーメール無効
```

---

## MyDNS

[MyDNS.JP](https://www.mydns.jp) への更新設定。エントリは `MYDNS_0_*`、`MYDNS_1_*`、`MYDNS_2_*` ... と増やせます。

### エントリキー（`N` = 0, 1, 2, ...）

| キー | 型 | 必須 | デフォルト | 説明 |
|------|----|------|-----------|------|
| `MYDNS_N_ID` | string | ✓ | — | MyDNS マスター ID（省略するとそのエントリを終端とみなす） |
| `MYDNS_N_PASS` | string | ✓ | — | MyDNS パスワード |
| `MYDNS_N_DOMAIN` | string | | `""` | ドメイン名（現在はログ用途のみ） |
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
ゾーン名からゾーン ID を自動解決します（ゾーン ID を直接書く必要はありません）。

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
`EMAIL_CHK_DDNS` または `EMAIL_UP_DDNS` を `on` にする場合、`EMAIL_ADR` は必須です。

| キー | 型 | デフォルト | 説明 |
|------|----|-----------|------|
| `EMAIL_CHK_DDNS` | bool | `off` | IP 変化時に通知 |
| `EMAIL_UP_DDNS` | bool | `off` | 定期更新時に通知 |
| `EMAIL_ADR` | string | `""` | 通知先メールアドレス |

```conf
EMAIL_CHK_DDNS=on
EMAIL_UP_DDNS=off
EMAIL_ADR=admin@example.com
```

> **Note:** メール送信は `sendmail` コマンドに依存します。事前にメール送信環境を構築してください。
