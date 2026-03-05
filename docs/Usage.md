# Usage

## コマンド一覧

```
dipper_ai <command>
```

| コマンド | 用途 | 通常の起動元 |
|----------|------|------------|
| `daemon` | 常駐プロセスとして起動（通常運用） | systemd |
| `update` | IP を取得し、変化があれば DDNS を更新 | daemon 内部 / 手動 |
| `check` | DNS 解決と現在 IP を比較し、不一致なら update を強制 | daemon 内部 / 手動 |
| `keepalive` | 全 MyDNS エントリを無条件で強制更新 | daemon 内部 / 手動 |
| `err_mail` | エラーログを集計し、条件を満たす場合にメール送信 | daemon 内部 / 手動 |

コマンドを省略した場合、または未知のコマンドを指定した場合は usage を stderr に出力して exit 1 します。

---

## daemon

常駐プロセスとして起動し、2 本の内部タイマーで定期処理を実行します。
通常は systemd によって起動・管理されます。

```bash
dipper_ai daemon
```

### 内部タイマー

| タイマー | 間隔設定 | 実行内容 |
|---------|---------|---------|
| check ticker | `DDNS_TIME` | `update` → `check` → `err_mail` の順に実行 |
| keepalive ticker | `UPDATE_TIME` | `keepalive`（`UPDATE_TIME=0` の場合は無効） |

- 起動直後に 10 秒待機（ネットワーク確立猶予）してから初回サイクルを即時実行します
- `SIGTERM` / `SIGINT` を受け取るとクリーンシャットダウンします

### ログ確認

```bash
# リアルタイムログ
journalctl -u dipper_ai -f

# 直近 50 行
journalctl -u dipper_ai -n 50

# 今日のログ
journalctl -u dipper_ai --since today
```

---

## update

現在のグローバル IP を取得し、前回送信値と異なる場合に DDNS プロバイダへ更新リクエストを送ります。

### 動作フロー

1. IPv4 / IPv6 を取得（`IPV4` / `IPV6` 設定に従う）
2. ドメインごとのキャッシュ（`cache_<entryKey>_<family>`）と比較
3. 変化したエントリのみ更新リクエストを送信
4. 成功したエントリのキャッシュを更新
5. `EMAIL_CHK_DDNS=on` の場合はメール通知

### 出力

```
dipper_ai update: IPv4=203.0.113.42
dipper_ai update: mydns[0] home.example.com ipv4: ok
```

IP が変化していない場合は**何も出力しません**（journal がクリーンに保たれます）。

---

## check

DNS 解決（`net.LookupHost`）で各ドメインの実際の IP を確認し、現在のグローバル IP と一致しない場合に対象エントリのキャッシュをリセットします。
キャッシュがリセットされると次の `update` 実行時に強制再送信が行われます。

### 動作フロー

1. 現在のグローバル IP を取得
2. 各ドメインを DNS 解決
3. 不一致を検知 → 対象エントリのキャッシュを削除
4. 不一致があった場合は `update` を呼び出して即時修正

### 出力

```
dipper_ai check: home.example.com A=1.1.1.1 want=203.0.113.42 → mismatch
dipper_ai check: mismatch detected — forcing DDNS update for affected domains
```

全ドメインが一致している場合は**何も出力しません**。

---

## keepalive

全 MyDNS エントリへ IP を問わず強制更新リクエストを送ります。
Cloudflare は API 登録が失効しないため対象外です。

```bash
# デーモン経由（UPDATE_TIME タイマー）で自動実行されるが手動でも可
dipper_ai keepalive
```

### 動作フロー

1. 現在のグローバル IP を取得
2. 全 MyDNS エントリへ無条件で送信（キャッシュの差分チェックなし）
3. 送信後にドメインキャッシュを更新
4. `EMAIL_UP_DDNS=on` の場合はメール通知

### 出力

```
dipper_ai keepalive: IPv4=203.0.113.42
dipper_ai keepalive: mydns[0] home.example.com ipv4: ok
```

---

## err_mail

`STATE_DIR` に蓄積されたエラーログを確認し、設定された条件でメール通知します。

- `ERR_CHK_TIME=0` または `EMAIL_ADR` が空の場合は何もせず exit 0
- エラーログが存在しない場合は何もしません
- 送信後はエラーログをクリアします

---

## systemd による管理

インストールスクリプトで以下のサービスが配置されます。

### dipper_ai.service

```ini
[Service]
Type=simple
ExecStart=/usr/bin/dipper_ai daemon
Restart=on-failure
RestartSec=30s
```

`Type=simple` のため systemd がプロセスの lifetime を管理します。
クラッシュ時は `RestartSec=30s` 後に自動再起動します。

### 操作コマンド

```bash
# サービス状態確認
systemctl status dipper_ai

# ログ確認（リアルタイム）
journalctl -u dipper_ai -f

# 手動で即時 keepalive 実行
dipper_ai keepalive

# 手動で即時チェック実行
dipper_ai check

# サービス再起動（設定変更後など）
systemctl restart dipper_ai

# サービス停止
systemctl stop dipper_ai

# 自動起動無効化
systemctl disable dipper_ai
```

---

## 多重起動防止

同一コマンドの二重起動を防ぐため、`STATE_DIR` にロックファイル（`lock_<command>`）を使用します。
すでに実行中の場合は `already running` を stderr に出力して **exit 0** します（エラー扱いにしない）。

---

## 設定ファイルのパス変更

```bash
DIPPER_AI_CONFIG=/path/to/custom.conf dipper_ai update
```

テスト環境や複数インスタンスの管理に利用できます。
