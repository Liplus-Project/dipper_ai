# dipper_ai

**dipper_ai** は AlmaLinux 9/10 向けの DDNS 自動更新ツールです。
IP アドレスの変化を検知し、MyDNS または Cloudflare の DNS レコードをリアルタイムで更新します。
systemd タイマーで定期実行し、エラー発生時にはメール通知を送ることができます。

---

## ページ一覧

| ページ | 内容 |
|--------|------|
| [Installation](Installation) | ビルド・インストール・アンインストール |
| [Configuration](Configuration) | `user.conf` 全キー一覧とデフォルト値 |
| [Usage](Usage) | コマンド仕様と systemd 運用 |
| [Architecture](Architecture) | 内部設計・パッケージ構成・データフロー |

---

## 特徴

- **マルチプロバイダ**: MyDNS と Cloudflare を同時に複数エントリ管理
- **IPv4 / IPv6 独立制御**: A レコードと AAAA レコードをエントリごとに ON/OFF
- **タイムゲート**: 設定した間隔ごとにのみ実行（不要な API 呼び出しを防止）
- **状態管理**: IP キャッシュ・エラーログをファイルで永続化
- **外部依存最小**: 標準 Go ライブラリのみ、`dig` 等の外部コマンド不使用

---

## クイックスタート

```bash
# 1. ビルド
go build -o dipper_ai ./cmd/dipper_ai

# 2. インストール（root 必要）
sudo bash scripts/install.sh

# 3. 設定
sudo cp /etc/dipper_ai/user.conf.example /etc/dipper_ai/user.conf
sudo vi /etc/dipper_ai/user.conf

# 4. 動作確認
dipper_ai check
```

詳細は [Installation](Installation) を参照してください。
