# Outlook Manager MCP 使用指南

`outlook-mcp` 是 Outlook Manager 的 MCP（Model Context Protocol）入口，通过 stdio 把 Outlook 邮件、日历、联系人和收件箱规则能力暴露给支持 MCP 的 AI 客户端。

本文覆盖：

- 构建与登录
- MCP 客户端配置
- 62 个 MCP tools 的分组说明
- 常用调用示例
- 返回格式、故障排查和安全说明

## 1. 前置条件

- Windows 10/11
- 已构建 `outlook-mcp.exe`
- 已使用同一个 Windows 用户完成 `outlookctl auth login`
- MCP 客户端与 `outlook-mcp.exe` 运行在同一个 Windows 用户下

MCP server 本身不提供登录交互。未登录时，tools 会返回认证或 token 刷新错误。

## 2. 构建

在仓库根目录执行：

```powershell
go build -trimpath -ldflags "-s -w" -o outlookctl.exe ./cmd/outlookctl
go build -trimpath -ldflags "-s -w" -o outlook-mcp.exe ./cmd/outlook-mcp
```

Windows amd64 发布构建：

```powershell
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-s -w" -o outlookctl.exe ./cmd/outlookctl
go build -trimpath -ldflags "-s -w" -o outlook-mcp.exe ./cmd/outlook-mcp
```

## 3. 登录

先使用 CLI 完成 Device Code 登录：

```powershell
.\outlookctl.exe auth login
.\outlookctl.exe auth status
```

登录成功后，本地会生成：

```text
%USERPROFILE%\.outlook-manager\config.json
%USERPROFILE%\.outlook-manager\accounts.json
%USERPROFILE%\.outlook-manager\token-cache.bin
```

其中：

- `config.json` 保存默认账号和 scopes
- `accounts.json` 保存可读账号档案，不包含 token
- `token-cache.bin` 使用 Windows DPAPI 加密

MCP server 只读取上述本地文件，不会接收或输出明文密码。

## 4. 配置 MCP 客户端

### 4.1 Claude Desktop

编辑：

```text
%APPDATA%\Claude\claude_desktop_config.json
```

使用构建好的 exe：

```json
{
  "mcpServers": {
    "outlook-manager": {
      "command": "C:\\path\\to\\outlook-mcp.exe",
      "args": []
    }
  }
}
```

保存后完全退出并重新启动 Claude Desktop。

### 4.2 Cline / Roo Code / Continue 等客户端

不同客户端的配置位置不同，但通常都接受相同的 stdio server 结构：

```json
{
  "mcpServers": {
    "outlook-manager": {
      "command": "C:\\path\\to\\outlook-mcp.exe",
      "args": [],
      "disabled": false
    }
  }
}
```

如果客户端支持环境变量，建议显式设置：

```json
{
  "env": {
    "USERPROFILE": "C:\\Users\\your-name"
  }
}
```

通常不需要设置，MCP server 会继承当前 Windows 用户环境。

### 4.3 从源码运行

部分客户端支持 `cwd`：

```json
{
  "mcpServers": {
    "outlook-manager": {
      "command": "go",
      "args": ["run", "./cmd/outlook-mcp"],
      "cwd": "D:\\project\\outlook_manager"
    }
  }
}
```

生产使用建议构建 exe，避免客户端启动时重复编译。

### 4.4 MCP Inspector

可以使用 MCP Inspector 检查 tools：

```powershell
npx @modelcontextprotocol/inspector C:\path\to\outlook-mcp.exe
```

## 5. 账号选择

大多数 tools 支持可选参数：

```json
{
  "account": "zero-67@hotmail.com"
}
```

也可以传入 HomeAccountID。不填时使用 `config.json` 中的默认账号。

建议在自动化场景中显式传入 `account`，避免切换默认账号后行为变化。

## 6. 工具清单

MCP 客户端会通过 `tools/list` 自动获取完整 JSON Schema。下面按能力分组列出 62 个 tools 及主要参数。

### 6.1 账号

| Tool | 说明 | 主要参数 |
| --- | --- | --- |
| `account_list` | 列出本地已登录账号档案 | 无 |
| `auth_status` | 检查 token 缓存并尝试静默刷新 | 无 |

### 6.2 邮件读取与查询

| Tool | 说明 | 主要参数 |
| --- | --- | --- |
| `mail_list` | 读取收件箱或指定文件夹邮件 | `account`、`folder`、`top`、`all`、`max` |
| `mail_read` | 读取单封邮件详情 | `id`、`account` |
| `mail_search` | 关键字搜索邮件 | `query`、`folder`、`top`、`account` |
| `mail_folders_list` | 列出邮件文件夹 | `top`、`account` |
| `mail_attachments_list` | 列出邮件附件 | `id`、`account` |
| `mail_attachment_save` | 下载邮件附件到本地 | `id`、`attachmentId`、`outputPath`、`account` |
| `mail_message_categories` | 列出单封邮件的类别 | `id`、`account` |
| `mail_rules_list` | 列出收件箱规则 | `account` |
| `mail_master_categories_list` | 列出主类别 | `account` |

### 6.3 邮件写入与操作

| Tool | 说明 | 主要参数 |
| --- | --- | --- |
| `mail_send` | 发送邮件 | `to`、`cc`、`bcc`、`subject`、`body`、`isHtml`、`attachments`、`importance`、`requestReadReceipt`、`requestDeliveryReceipt`、`account` |
| `mail_reply` | 回复邮件 | `id`、`body`、`account` |
| `mail_reply_all` | 回复全部收件人 | `id`、`body`、`account` |
| `mail_forward` | 转发邮件 | `id`、`to`、`body`、`account` |
| `mail_draft_create` | 创建草稿 | 与 `mail_send` 类似，不含 `saveToSentItems` |
| `mail_draft_update` | 更新草稿 | `id`、`to`、`subject`、`body`、`attachments`、`importance`、回执参数、`account` |
| `mail_draft_send` | 发送草稿 | `id`、`account` |
| `mail_drafts_list` | 列出草稿 | `top`、`account` |
| `mail_mark_read` | 标记已读 | `id`、`account` |
| `mail_mark_unread` | 标记未读 | `id`、`account` |
| `mail_flag` | 设置旗标 | `id`、`account` |
| `mail_unflag` | 取消旗标 | `id`、`account` |
| `mail_set_importance` | 设置重要性 | `id`、`level`、`account` |
| `mail_move` | 移动到文件夹 | `id`、`folder`、`account` |
| `mail_delete` | 移动到已删除邮件 | `id`、`account` |
| `mail_permanent_delete` | 永久删除邮件 | `id`、`account` |
| `mail_category_add` | 给邮件添加类别 | `id`、`category`、`account` |
| `mail_category_remove` | 移除邮件类别 | `id`、`category`、`account` |
| `mail_folder_create` | 创建邮件文件夹 | `name`、`parent`、`account` |
| `mail_folder_rename` | 重命名邮件文件夹 | `id`、`name`、`account` |
| `mail_folder_delete` | 删除邮件文件夹 | `id`、`account` |
| `mail_rule_create` | 创建收件箱规则 | `name`、`from`、`subjectContains`、`moveTo`、`markAsRead`、`delete`、`stop`、`disabled`、`account` |
| `mail_rule_update` | 更新收件箱规则 | `id` + 创建规则参数 |
| `mail_rule_delete` | 删除收件箱规则 | `id`、`account` |
| `mail_master_category_create` | 创建主类别 | `name`、`color`、`account` |
| `mail_master_category_update` | 更新主类别颜色 | `id`、`color`、`account` |
| `mail_master_category_delete` | 删除主类别 | `id`、`account` |

说明：Microsoft Graph 不支持修改主类别名称，只支持修改颜色。

### 6.4 日历

| Tool | 说明 | 主要参数 |
| --- | --- | --- |
| `calendar_list` | 读取未来指定天数的事件 | `days`、`top`、`calendar`、`account` |
| `calendar_calendars_list` | 列出日历 | `account` |
| `calendar_create` | 创建日历事件 | `subject`、`start`、`end`、`body`、`location`、`timeZone`、`isAllDay`、`attendees`、`reminderMinutes`、重复参数、`categories`、`showAs`、`importance`、`sensitivity`、`account` |
| `calendar_update` | 更新日历事件 | `id` + 创建事件参数 |
| `calendar_delete` | 删除日历事件 | `id`、`account` |
| `calendar_cancel` | 取消日历事件 | `id`、`comment`、`account` |
| `calendar_respond` | 响应日历邀请 | `id`、`response`（accept/decline/tentative）、`comment`、`account` |
| `calendar_attachments_list` | 列出事件附件 | `id`、`account` |
| `calendar_attachment_add` | 给事件添加附件 | `id`、`file`、`account` |
| `calendar_attachment_get` | 下载事件附件 | `id`、`attachmentId`、`account` |

说明：`calendar_respond` 只能用于别人发来的会议邀请；如果当前账号是会议组织者，Graph 会拒绝响应。`calendar_cancel` 通常也需要当前账号是会议组织者。

### 6.5 联系人

| Tool | 说明 | 主要参数 |
| --- | --- | --- |
| `contacts_list` | 读取联系人列表 | `folder`、`top`、`account` |
| `contacts_search` | 按显示名称前缀搜索 | `query`、`top`、`account` |
| `contacts_create` | 创建联系人 | `displayName`、`givenName`、`surname`、`emails`、`companyName`、`jobTitle`、`mobilePhone`、`businessPhones`、`nickName`、`birthday`、`personalNotes`、`department`、`officeLocation`、`profession`、`businessHomePage`、地址对象、`folder`、`account` |
| `contacts_update` | 更新联系人 | `id` + 创建联系人参数 |
| `contacts_delete` | 删除联系人 | `id`、`account` |
| `contacts_permanent_delete` | 永久删除联系人 | `id`、`account` |
| `contacts_copy` | 复制联系人到文件夹 | `id`、`folder`、`name`、`account` |
| `contacts_move` | 移动联系人到文件夹 | `id`、`folder`、`account` |
| `contacts_folders_list` | 列出联系人文件夹 | `top`、`account` |
| `contacts_folder_create` | 创建联系人文件夹 | `name`、`parent`、`account` |
| `contacts_folder_rename` | 重命名联系人文件夹 | `id`、`name`、`account` |
| `contacts_folder_delete` | 删除联系人文件夹 | `id`、`account` |
| `contacts_photo_get` | 读取联系人照片 | `id`、`account` |
| `contacts_photo_set` | 设置联系人照片 | `id`、`file`、`account` |

## 7. 常用示例

以下示例展示 MCP 客户端调用 tools 时的名称和 arguments，不依赖特定客户端。

### 7.1 查看最近邮件

```json
{
  "tool": "mail_list",
  "arguments": {
    "top": 5
  }
}
```

跨页拉取最多 300 封：

```json
{
  "tool": "mail_list",
  "arguments": {
    "all": true,
    "max": 300
  }
}
```

### 7.2 搜索并读取邮件

```json
{
  "tool": "mail_search",
  "arguments": {
    "query": "发票",
    "top": 5
  }
}
```

拿到邮件 ID 后：

```json
{
  "tool": "mail_read",
  "arguments": {
    "id": "<message-id>"
  }
}
```

### 7.3 发送带附件的邮件

```json
{
  "tool": "mail_send",
  "arguments": {
    "to": ["someone@example.com"],
    "subject": "项目资料",
    "body": "请查收附件。",
    "attachments": ["C:\\temp\\资料.pdf"],
    "importance": "high",
    "requestReadReceipt": true
  }
}
```

附件路径是 MCP server 所在 Windows 机器上的本地路径，不是 AI 客户端所在的远程路径。

附件支持 Graph 官方的大文件上传流程：单文件不超过 3 MiB 时使用简单附件；3 MiB 到 150 MiB 之间会自动创建草稿、创建上传会话、分片上传，然后发送草稿。

### 7.4 创建、更新并发送草稿

```json
{
  "tool": "mail_draft_create",
  "arguments": {
    "to": ["someone@example.com"],
    "subject": "草稿主题",
    "body": "第一版正文"
  }
}
```

```json
{
  "tool": "mail_draft_update",
  "arguments": {
    "id": "<draft-id>",
    "to": ["someone@example.com"],
    "subject": "更新后的主题",
    "body": "更新后的正文",
    "importance": "high"
  }
}
```

```json
{
  "tool": "mail_draft_send",
  "arguments": {
    "id": "<draft-id>"
  }
}
```

### 7.5 创建日历事件

```json
{
  "tool": "calendar_create",
  "arguments": {
    "subject": "项目周会",
    "start": "2026-11-02T10:00:00",
    "end": "2026-11-02T11:00:00",
    "timeZone": "Asia/Shanghai",
    "attendees": ["someone@example.com"],
    "reminderMinutes": 10,
    "categories": ["项目A", "重要"],
    "showAs": "busy",
    "importance": "high"
  }
}
```

### 7.6 给日历事件添加附件

```json
{
  "tool": "calendar_attachment_add",
  "arguments": {
    "id": "<event-id>",
    "file": "C:\\temp\\会议资料.pdf"
  }
}
```

### 7.7 创建联系人

```json
{
  "tool": "contacts_create",
  "arguments": {
    "displayName": "张三",
    "emails": ["zhangsan@example.com"],
    "nickName": "老张",
    "department": "研发部",
    "officeLocation": "A栋101",
    "birthday": "1990-01-02",
    "homeAddress": {
      "street": "家庭路1号",
      "city": "上海",
      "state": "上海",
      "postalCode": "200000",
      "countryOrRegion": "中国"
    }
  }
}
```

### 7.8 给联系人设置照片

```json
{
  "tool": "contacts_photo_set",
  "arguments": {
    "id": "<contact-id>",
    "file": "C:\\temp\\photo.jpg"
  }
}
```

### 7.9 创建禁用状态的收件箱规则

```json
{
  "tool": "mail_rule_create",
  "arguments": {
    "name": "测试规则",
    "subjectContains": ["测试关键字"],
    "markAsRead": true,
    "disabled": true
  }
}
```

### 7.10 更新主类别颜色

```json
{
  "tool": "mail_master_category_update",
  "arguments": {
    "id": "<category-id>",
    "color": "preset5"
  }
}
```

## 8. 返回格式

大多数 tools 返回 JSON 文本，外层通常包含：

```json
{
  "success": true,
  "account": "zero-67@hotmail.com"
}
```

不同 tool 会附加自己的字段，例如：

- `mail_list` 返回 `messages`
- `mail_read` 返回邮件详情
- `calendar_list` 返回 `events`
- `contacts_list` 返回 `contacts`
- `contacts_photo_get` 和 `calendar_attachment_get` 返回 `base64` 和 `contentType`

ID 都是 Graph 返回的不透明字符串，必须原样传递，不要自行截断或 URL 编码。

## 9. 故障排查

### 9.1 `auth_status` 显示刷新失败

重新登录：

```powershell
.\outlookctl.exe auth login
```

### 9.2 `invalid_grant` 或 scopes 未授权

通常是 refresh token 失效或新增 scope 后尚未重新授权。重新执行 Device Code 登录并确认权限页面。

### 9.3 HTTP 403

检查 scopes 是否包含所需权限。常见权限：

```text
User.Read
Mail.Read
Mail.ReadWrite
Mail.Send
MailboxSettings.ReadWrite
Calendars.Read
Calendars.ReadWrite
Contacts.Read
Contacts.ReadWrite
```

### 9.4 网络超时

Graph 请求层会自动重试临时网络错误、HTTP 429 和 5xx。若仍失败，检查代理、VPN 和网络连通性。

### 9.5 本地文件路径错误

`attachments`、`file`、`outputPath` 等路径都相对于 MCP server 所在机器解析。建议使用绝对路径。

## 10. 安全说明

- token-cache.bin 使用 Windows DPAPI 加密，只能由同一 Windows 用户解密
- accounts.json 是可读账号元数据，不包含 token
- 不要把 token-cache.bin 提交到仓库或发送给他人
- MCP tools 可以读取本机指定路径并上传为附件，请只连接可信的 MCP 客户端
- 永久删除、取消会议、发送邮件等操作不可逆，建议在自动化流程中增加人工确认

## 11. 发布

GitHub Actions 在推送 `v*` tag 时构建 Windows amd64 二进制：

```text
outlookctl-windows-amd64.exe
outlook-mcp-windows-amd64.exe
```

发布前需要确认 tag 对应提交已经在 `main` 分支上。