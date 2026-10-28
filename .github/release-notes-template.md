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

## 未签名说明

本项目的 Release 不提供 Windows Authenticode 或 GPG 签名。

Windows 可能显示 `Unknown publisher` 或 SmartScreen 提示，请以 SHA256SUMS 校验结果为准。

## 兼容性

- Windows 使用 DPAPI 保存 token。
- Linux 需要可用的 Secret Service（GNOME Keyring、KWallet 等）。
- macOS 使用系统 Keychain。

## 变更

