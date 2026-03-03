# Installation

## 必要環境

| 項目 | 要件 |
|------|------|
| OS | AlmaLinux 9 / 10（他の systemd Linux でも動作可能） |
| Go | 1.23 以上（ビルド時のみ） |
| git | リポジトリのクローンに必要 |
| 権限 | インストールスクリプトは root 必要 |

### 依存パッケージのインストール（AlmaLinux）

```bash
# git
sudo dnf install -y git

# Go 1.23+（dnf のバージョンが古い場合は公式バイナリを使用）
sudo dnf install -y golang
go version  # 1.23 未満の場合は以下の手順で公式バイナリを導入

# Go 公式バイナリ（dnf で 1.23+ が入らない場合）
curl -LO https://go.dev/dl/go1.23.6.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.23.6.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' | sudo tee /etc/profile.d/go.sh
source /etc/profile.d/go.sh
go version
```

---

## ビルド

```bash
git clone https://github.com/Liplus-Project/dipper_ai.git
cd dipper_ai
go build -o dipper_ai ./cmd/dipper_ai
```

クロスコンパイル例（Linux AMD64 向け）:

```bash
GOOS=linux GOARCH=amd64 go build -o dipper_ai ./cmd/dipper_ai
```

---

## インストール

`scripts/install.sh` を root で実行します。
以下のファイルが配置され、systemd タイマーが有効化されます。

```bash
sudo bash scripts/install.sh
```

### 配置先

| ファイル | 配置先 |
|----------|--------|
| `dipper_ai` バイナリ | `/usr/bin/dipper_ai` |
| 設定サンプル | `/etc/dipper_ai/user.conf.example` |
| systemd サービス | `/etc/systemd/system/dipper_ai.service` |
| systemd タイマー | `/etc/systemd/system/dipper_ai.timer` |
| 状態ファイル | `/etc/dipper_ai/state/`（実行時に自動生成） |

### インストール後の手順

```bash
# 設定ファイルを作成して編集
sudo cp /etc/dipper_ai/user.conf.example /etc/dipper_ai/user.conf
sudo vi /etc/dipper_ai/user.conf

# タイマーの状態確認
systemctl status dipper_ai.timer

# 手動で update を実行してテスト
sudo dipper_ai update
```

---

## アンインストール

```bash
sudo bash scripts/uninstall.sh
```

サービス・タイマーの停止と無効化、バイナリの削除を行います。
設定ファイル (`/etc/dipper_ai/`) は保持されます。手動で削除してください。

---

## 設定ファイルのパス

デフォルトは `/etc/dipper_ai/user.conf` ですが、環境変数 `DIPPER_AI_CONFIG` で上書きできます。

```bash
DIPPER_AI_CONFIG=/path/to/custom.conf dipper_ai update
```

---

## テスト実行

```bash
go test ./...
```

全パッケージのユニットテスト・受け入れテストが実行されます（外部 API 接続不要）。
