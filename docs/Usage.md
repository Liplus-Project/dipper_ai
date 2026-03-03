# Usage

## コマンド

```
dipper_ai <command>
```

| コマンド | 説明 |
|----------|------|
| `update` | 現在の IP を取得し、変化していれば DDNS を更新する |
| `check` | 現在の IP と DDNS 状態を stdout に出力する |
| `err_mail` | 蓄積されたエラーを確認し、条件を満たす場合にメール送信する |

コマンドを省略した場合、または未知のコマンドを指定した場合は usage を stderr に出力して exit 1 します。

---

## update

IP アドレスを取得し、前回から変化があれば DDNS プロバイダに更新リクエストを送ります。

**タイムゲート**: `UPDATE_TIME` 分以内に再実行された場合はスキップします（exit 0）。
**DDNS ゲート**: 更新後 `DDNS_TIME` 分以内に再更新が抑制されます。

### 動作フロー

1. `gate_update` タイムゲートを確認 → 未経過ならスキップ
2. 現在の IPv4 / IPv6 を取得（`IPV4` / `IPV6` 設定に従う）
3. 前回の IP と比較
4. 変化あり → 各 MyDNS エントリ・各 Cloudflare エントリへ更新リクエスト
5. 結果を状態ファイルに保存

### 出力例

```
$ dipper_ai update
# 変化なし: 出力なし、exit 0
# IP 変化あり: 各プロバイダへの更新が実行される
```

---

## check

現在保持している IP アドレスを stdout に出力します。
IP キャッシュが有効（`IP_CACHE_TIME > 0`）かつキャッシュが新鮮な場合は、状態ファイルの値をそのまま使用します。

**タイムゲート**: `DDNS_TIME` 分以内の再実行はスキップします（exit 0）。

### 出力例

```
$ dipper_ai check
ipv4: 203.0.113.42
ipv6: 2001:db8::1
```

IPv6 が無効（`IPV6=off`）の場合は `ipv6:` 行は出力されません。

---

## err_mail

`STATE_DIR` に蓄積されたエラーログを確認し、設定された条件で通知メールを送ります。

**無効条件**: `ERR_CHK_TIME=0` または `EMAIL_ADR` が空の場合は何もせず exit 0。
**タイムゲート**: `ERR_CHK_TIME` 分以内の再実行はスキップします（exit 0）。

エラーログが存在しない場合は何もしません。送信後はエラーログをクリアします。

---

## systemd による自動実行

インストールスクリプトで以下の 2 つの unit が配置されます。

### dipper_ai.timer

```ini
[Timer]
OnBootSec=2min          # 起動 2 分後に初回実行
OnUnitActiveSec=5min    # 以降 5 分ごとに実行
```

### dipper_ai.service

```ini
[Service]
Type=oneshot
WorkingDirectory=/etc/dipper_ai
ExecStart=/usr/local/bin/dipper_ai update
ExecStartPost=/usr/local/bin/dipper_ai check
ExecStartPost=/usr/local/bin/dipper_ai err_mail
```

1 回の発火で `update` → `check` → `err_mail` が順番に実行されます。
各コマンドは自身のタイムゲートを持つため、5 分ごとに起動しても実際の更新は `UPDATE_TIME` 間隔でのみ行われます。

### 操作コマンド

```bash
# タイマー状態確認
systemctl status dipper_ai.timer

# 直近のログ確認
journalctl -u dipper_ai.service -n 50

# 手動で即時実行
systemctl start dipper_ai.service

# タイマー無効化（停止）
systemctl disable --now dipper_ai.timer

# タイマー再有効化
systemctl enable --now dipper_ai.timer
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
