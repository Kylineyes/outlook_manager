# 双账号日历验证计划

本文用于第二个测试账号到位后的验证，覆盖：

- 日历邀请响应
- 共享日历读取
- 响应状态回读

## 前置条件

准备两个 Microsoft 账号：

- 账号 A：会议组织者
- 账号 B：会议参与人

两个账号都使用同一个 Windows 用户登录到 Outlook Manager：

```powershell
.\outlookctl.exe auth login
```

重复执行并使用不同账号完成授权后，可用以下命令确认：

```powershell
.\outlookctl.exe account list
```

后续命令通过 `--account` 指定账号。

## 一、日历邀请响应

### 1. 账号 A 创建会议

```powershell
.\outlookctl.exe calendar create `
  --account A@example.com `
  --subject "双账号邀请测试" `
  --start "2026-11-10T10:00:00" `
  --end "2026-11-10T11:00:00" `
  --attendees B@example.com
```

记录输出中的事件 ID。

### 2. 账号 B 读取邀请

```powershell
.\outlookctl.exe calendar list --account B@example.com --days 30 --json
```

找到主题为 `双账号邀请测试` 的事件，确认：

```json
{
  "responseStatus": {
    "response": "notResponded"
  }
}
```

### 3. 账号 B 接受邀请

```powershell
.\outlookctl.exe calendar respond `
  --account B@example.com `
  --id <事件ID> `
  --response accept `
  --comment "接受测试"
```

### 4. 账号 B 回读响应状态

```powershell
.\outlookctl.exe calendar list --account B@example.com --days 30 --json
```

确认 `responseStatus.response` 变为：

```text
accepted
```

### 5. 账号 B 测试拒绝和暂定

重复创建会议后分别执行：

```powershell
.\outlookctl.exe calendar respond --account B@example.com --id <事件ID> --response decline
.\outlookctl.exe calendar respond --account B@example.com --id <事件ID> --response tentative
```

预期状态：

```text
declined
tentativelyAccepted
```

### 6. 清理

账号 A 删除测试事件：

```powershell
.\outlookctl.exe calendar delete --account A@example.com --id <事件ID>
```

## 二、共享日历验证

### 1. 在 Outlook Web 中共享日历

由账号 B 在 Outlook Web 中把日历共享给账号 A，或反向共享。

### 2. 账号 A 列出日历

```powershell
.\outlookctl.exe calendar calendars --account A@example.com --json
```

检查是否出现共享日历。

### 3. 读取共享日历事件

```powershell
.\outlookctl.exe calendar list `
  --account A@example.com `
  --calendar "<共享日历名称或ID>" `
  --days 30
```

### 4. 当前限制

如果共享日历没有出现在 `/me/calendars` 中，说明当前实现只覆盖默认日历和用户自有日历。后续需要增加：

- 共享日历发现
- 委托邮箱日历
- 其他用户日历读取
- 对应权限申请

## 三、审计日志

每次写操作会记录到：

```text
%USERPROFILE%\.outlook-manager\logs\audit.log
```

例如：

```json
{"action":"calendar.respond","eventId":"...","response":"accept","time":"..."}
```

审计日志不会记录邮件正文、附件内容或 token。

## 四、当前已知限制

- 自己创建的会议不能由自己响应，Graph 会返回 organizer 错误。
- 共享日历是否出现在 `calendar calendars` 取决于账号共享配置和 Graph 返回结果。
- 第二个账号需要用户自行提供并完成 Device Code 授权。