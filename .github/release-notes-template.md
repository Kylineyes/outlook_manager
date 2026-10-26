# Outlook Manager {{TAG}}

发布日期：{{DATE}}

## 安装

解压对应平台的压缩包：

- Windows amd64：`outlook-manager-windows-amd64-x64-{{TAG}}.zip`
- Windows arm64：`outlook-manager-windows-arm64-{{TAG}}.zip`
- Linux amd64：`outlook-manager-linux-amd64-x64-{{TAG}}.zip`
- Linux arm64：`outlook-manager-linux-arm64-{{TAG}}.zip`

压缩包内包含：

- `outlookctl` / `outlookctl.exe`
- `outlook-mcp` / `outlook-mcp.exe`
- `README.md`
- `LICENSE`
- `CHANGELOG.md`

## 校验

下载 `SHA256SUMS` 后执行：

```text
sha256sum -c SHA256SUMS
```

如果发布包含 GPG 签名，可执行：

```text
gpg --verify SHA256SUMS.asc SHA256SUMS
```

## 代码签名

- Windows 二进制在配置了 Authenticode 证书时会进行 SHA-256 签名。
- Linux/通用校验文件在配置了 GPG 私钥时会生成 `SHA256SUMS.asc`。
- 如果本次发布未配置签名密钥，压缩包仍会提供 SHA256 校验和。

## 兼容性

- Windows 使用 DPAPI 保存 token。
- Linux 需要可用的 Secret Service（GNOME Keyring、KWallet 等）。
- macOS 使用系统 Keychain。

## 变更

