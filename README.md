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

```sh
# Build
go build -o dipper_ai ./cmd/dipper_ai

# Install (as root, AlmaLinux 9/10)
sudo ./scripts/install.sh

# Uninstall
sudo ./scripts/uninstall.sh
```

## Development Status

This project is under active development toward Li+ v1.0.0.
See [issue #8](https://github.com/Liplus-Project/liplus-language/issues/8) for the acceptance criteria.

## License

MIT
