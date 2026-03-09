# dipper_ai

A Li+-guided implementation of [dipper](https://github.com/Liplus-Project/liplus-language/issues/8) —
a DDNS manager for MyDNS and Cloudflare, targeting AlmaLinux 9/10.

## Goals

`dipper_ai` aims to achieve **functional parity with dipper** as defined by
[liplus-language issue #8](https://github.com/Liplus-Project/liplus-language/issues/8).
Parity is judged entirely on observable output (state files, stdout/stderr, exit code).

## Commands

```sh
dipper_ai update    # Fetch IP, update DDNS if changed
dipper_ai check     # Report current IP and DDNS status
dipper_ai err_mail  # Aggregate errors and notify if threshold met
```

## Configuration

Copy `user.conf.example` to `/etc/dipper_ai/user.conf` and fill in your credentials.
Configuration format is intentionally independent from dipper (see spec: MAY differ).

## Install / Uninstall

**リリースバイナリ（Go不要）:**

```sh
# 例: Linux AMD64
curl -LO https://github.com/Liplus-Project/dipper_ai/releases/latest/download/dipper_ai_linux_amd64.tar.gz
tar -xzf dipper_ai_linux_amd64.tar.gz
sudo mv dipper_ai /usr/bin/dipper_ai
```

**go install:**

```sh
go install github.com/Liplus-Project/dipper_ai/cmd/dipper_ai@latest
```

**ソースからビルド:**

```sh
go build -o dipper_ai ./cmd/dipper_ai
```

**systemdセットアップ（AlmaLinux 9/10）:**

```sh
# Install (as root)
sudo ./scripts/install.sh

# Uninstall
sudo ./scripts/uninstall.sh
```

詳細は [Installation.md](Installation.md) を参照。

## Development Status

This project is under active development toward Li+ v1.0.0.
See [issue #8](https://github.com/Liplus-Project/liplus-language/issues/8) for the acceptance criteria.

## License

MIT
