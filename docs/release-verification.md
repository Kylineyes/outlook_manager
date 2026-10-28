# 发布校验说明

## 发布策略

本项目 Release 永久采用 unsigned 策略：

- 不提供 Windows Authenticode 代码签名
- 不提供 GPG 签名
- 不生成 `SHA256SUMS.asc` 或 `public-key.asc`
- 每个 Release 提供 `SHA256SUMS`

Windows 可能显示：

```text
Unknown publisher
```

SmartScreen 也可能给出提示。这是未签名发布的预期行为，不代表下载文件损坏。请使用 `SHA256SUMS` 校验文件完整性。

## 校验 SHA256SUMS

下载 Release 中的压缩包和 `SHA256SUMS` 后执行：

Linux/macOS：

```bash
sha256sum -c SHA256SUMS
```

Windows PowerShell：

```powershell
$sums = Get-Content .\SHA256SUMS
foreach ($line in $sums) {
  $parts = $line -split '\s+', 2
  $expected = $parts[0]
  $name = $parts[1].TrimStart('*')
  $actual = (Get-FileHash ".\$name" -Algorithm SHA256).Hash.ToLower()
  if ($actual -ne $expected.ToLower()) {
    throw "SHA256 校验失败: $name"
  }
}
```

## 压缩包内容

Windows：

```text
outlookctl.exe
outlook-mcp.exe
README.md
LICENSE
CHANGELOG.md
```

Linux：

```text
outlookctl
outlook-mcp
README.md
LICENSE
CHANGELOG.md
```

## 兼容性

- Windows 使用 DPAPI 保存 token。
- Linux 需要可用的 Secret Service（GNOME Keyring、KWallet 等）。
- macOS 使用系统 Keychain。