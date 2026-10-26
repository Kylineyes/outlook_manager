# 发布签名说明

## Windows Authenticode

发布 workflow 支持可选的 Windows 代码签名。需要在仓库 Secrets 中配置：

```text
WINDOWS_CERT_BASE64
WINDOWS_CERT_PASSWORD
```

`WINDOWS_CERT_BASE64` 是 PFX 证书文件的 Base64 编码。

本地生成示例：

```powershell
[Convert]::ToBase64String([IO.File]::ReadAllBytes("codesign.pfx"))
```

配置后 workflow 会：

1. 解码 PFX
2. 使用 `signtool.exe` 对 `outlookctl.exe` 和 `outlook-mcp.exe` 做 SHA-256 Authenticode 签名
3. 使用 DigiCert 时间戳服务
4. 执行 `signtool verify /pa`

未配置证书时 workflow 会跳过签名并输出提示。

## GPG 校验和签名

可选配置：

```text
GPG_PRIVATE_KEY
```

内容是 ASCII-armored GPG 私钥。

配置后 workflow 会：

1. 导入私钥
2. 生成 `SHA256SUMS.asc`
3. 导出 `public-key.asc`

如果未配置，Release 仍会包含 `SHA256SUMS`，但不包含 GPG 签名。

## 验证方式

Windows：

```powershell
Get-AuthenticodeSignature .\outlookctl.exe
Get-AuthenticodeSignature .\outlook-mcp.exe
```

校验和：

```bash
sha256sum -c SHA256SUMS
```

GPG：

```bash
gpg --import public-key.asc
gpg --verify SHA256SUMS.asc SHA256SUMS
```
