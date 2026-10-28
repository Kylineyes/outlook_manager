# Outlook Manager MCP 使用指南

`outlook-mcp` 是 Outlook Manager 的 MCP（Model Context Protocol）入口，通过 stdio 把 Outlook 邮件、日历、联系人和收件箱规则能力暴露给支持 MCP 的 AI 客户端。

本文覆盖：

- 构建与登录
- MCP 客户端配置
- 118 个 MCP tools 的分组说明
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
- `token-cache.bin` 使用系统安全存储加密（Windows DPAPI / Linux Secret Service / macOS Keychain）

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

MCP 客户端会通过 `tools/list` 自动获取完整 JSON Schema。下面按能力分组列出 118 个 tools 及主要参数。

### 6.1 账号

| Tool | 说明 | 主要参数 |
| --- | --- | --- |
| `account_list` | 列出本地已登录账号档案 | 无 |
| `auth_status` | 检查 token 缓存并尝试静默刷新 | 无 |
| `mailbox_settings_get` | 读取邮箱设置 | `account` |
| `mailbox_auto_reply_get` | 读取自动回复设置 | `account` |
| `mailbox_auto_reply_set` | 设置自动回复 | `status`、`internalMessage`、`externalMessage`、`externalAudience`、`start`、`end`、`timeZone`、`account` |
| `mailbox_timezone_get` | 读取邮箱时区 | `account` |
| `mailbox_timezone_set` | 设置邮箱时区 | `value`、`account` |
| `audit_list` | 查询本地审计日志 | `limit`、`action`、`account`、`query`、`since`、`until` |
| `audit_export` | 导出审计日志 | `file`、`format`、查询过滤参数 |
| `audit_stats` | 审计统计 | 查询过滤参数 |
| `audit_verify` | 校验审计哈希链 | 无 |
| `audit_config_get` | 读取审计配置 | 无 |
| `audit_config_set` | 设置审计配置 | `maxSizeMB`、`maxBackups`、`retentionDays` |
| `audit_cleanup` | 清理过期审计日志 | 无 |
| `update_check` | 检查 GitHub 最新发布版本 | 无 |

### 6.2 邮件读取与查询

| Tool | 说明 | 主要参数 |
| --- | --- | --- |
| `mail_list` | 读取收件箱或指定文件夹邮件 | `account`、`folder`、`top`、`all`、`max` |
| `mail_read` | 读取单封邮件详情 | `id`、`account` |
| `mail_search` | 关键字搜索邮件 | `query`、`folder`、`top`、`account` |
| `mail_query` | 高级邮件查询 | `folder`、`from`、`to`、`subject`、`body`、`search`、`importance`、`category`、`after`、`before`、`unreadOnly`、`hasAttachment`、`top`、`account` |
| `mail_view_save` | 保存邮件视图 | `name` + 高级查询参数 |
| `mail_view_list` | 列出邮件视图 | 无 |
| `mail_view_run` | 运行邮件视图 | `name`、`account` |
| `mail_view_delete` | 删除邮件视图 | `name` |
| `mail_folders_list` | 列出邮件文件夹 | `top`、`account` |
| `mail_folder_tree` | 树形列出邮件文件夹 | `top`、`account` |
| `mail_attachments_list` | 列出邮件附件 | `id`、`account` |
| `mail_attachment_save` | 下载邮件附件到本地 | `id`、`attachmentId`、`outputPath`、`account` |
| `mail_export_eml` | 导出邮件为 EML/MIME 文件 | `id`、`file`、`account` |
| `mail_import_eml` | 从 EML 导入为草稿，默认 dry-run | `file`、`confirm`、`account` |
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
| `mail_draft_attachment_delete` | 删除草稿附件 | `id`、`attachmentId`、`account` |
| `mail_draft_attachment_replace` | 替换草稿附件 | `id`、`attachmentId`、`file`、`account` |
| `mail_draft_attachment_bulk_delete` | 批量删除草稿附件 | `id`、`attachmentIds`、`confirm`、`account` |
| `mail_draft_send` | 发送草稿 | `id`、`account` |
| `mail_drafts_list` | 列出草稿 | `top`、`account` |
| `mail_mark_read` | 标记已读 | `id`、`account` |
| `mail_mark_unread` | 标记未读 | `id`、`account` |
| `mail_flag` | 设置旗标 | `id`、`account` |
| `mail_unflag` | 取消旗标 | `id`、`account` |
| `mail_set_importance` | 设置重要性 | `id`、`level`、`account` |
| `mail_move` | 移动到文件夹 | `id`、`folder`、`account` |
| `mail_delete` | 移动到已删除邮件 | `id`、`account` |
| `mail_bulk_move` | 批量移动邮件，默认 dry-run | `ids`、`folder`、`confirm`、`account` |
| `mail_bulk_delete` | 批量删除邮件，默认 dry-run | `ids`、`confirm`、`account` |
| `mail_bulk_mark_read` | 批量标记已读，默认 dry-run | `ids`、`confirm`、`account` |
| `mail_bulk_mark_unread` | 批量标记未读，默认 dry-run | `ids`、`confirm`、`account` |
| `mail_bulk_flag` | 批量设置旗标，默认 dry-run | `ids`、`confirm`、`account` |
| `mail_bulk_unflag` | 批量取消旗标，默认 dry-run | `ids`、`confirm`、`account` |
| `mail_bulk_set_importance` | 批量设置重要性，默认 dry-run | `ids`、`level`、`confirm`、`account` |
| `mail_bulk_category_add` | 批量添加类别，默认 dry-run | `ids`、`category`、`confirm`、`account` |
| `mail_bulk_category_remove` | 批量移除类别，默认 dry-run | `ids`、`category`、`confirm`、`account` |
| `mail_permanent_delete` | 永久删除邮件 | `id`、`account` |
| `mail_category_add` | 给邮件添加类别 | `id`、`category`、`account` |
| `mail_category_remove` | 移除邮件类别 | `id`、`category`、`account` |
| `mail_folder_create` | 创建邮件文件夹 | `name`、`parent`、`account` |
| `mail_folder_rename` | 重命名邮件文件夹 | `id`、`name`、`account` |
| `mail_folder_delete` | 删除邮件文件夹 | `id`、`account` |
| `mail_rule_create` | 创建收件箱规则 | `name`、`from`、`subjectContains`、`bodyContains`、`bodyOrSubjectContains`、`hasAttachments`、`importance`、`senderContains`、`recipientContains`、`moveTo`、`copyTo`、`assignCategories`、`forwardTo`、`redirectTo`、`markAsRead`、`markImportance`、`delete`、`permanentDelete`、`stop`、`disabled`、`applyNow`、`confirmApply`、`applyFolder`、`applyFolders`、`applyMax`、`account` |
| `mail_rule_update` | 更新收件箱规则 | `id` + 创建规则参数 |
| `mail_rule_apply` | 回放规则到已有邮件，默认 dry-run | `id`、`folder`、`folders`、`max`、`confirm`、`account` |
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
| `calendar_forward` | 转发日历事件 | `id`、`to`、`comment`、`account` |
| `calendar_export_ics` | 导出日历事件为 ICS | `file`、`calendar`、`max`、`account` |
| `calendar_import_ics` | 从 ICS 导入事件，默认 dry-run | `file`、`confirm`、`max`、`account` |
| `calendar_series_info` | 查看重复事件系列信息 | `id`、`account` |
| `calendar_series_update` | 更新单次事件或整个系列 | `id`、`scope`、`subject`、`start`、`end`、`timeZone`、`location`、`body`、`categories`、`showAs`、`importance`、`sensitivity`、`account` |
| `calendar_series_delete` | 删除单次事件或整个系列 | `id`、`scope`、`account` |
| `calendar_free_busy` | 查询空闲/忙碌状态 | `schedules`、`start`、`end`、`interval`、`account` |
| `calendar_find_meeting_times` | 查询共同空闲会议时间 | `attendees`、`start`、`end`、`duration`、`max`、`account` |
| `calendar_attachments_list` | 列出事件附件 | `id`、`account` |
| `calendar_attachment_add` | 给事件添加附件 | `id`、`file`、`account` |
| `calendar_attachment_get` | 下载事件附件 | `id`、`attachmentId`、`account` |
| `calendar_attachment_delete` | 删除事件附件 | `id`、`attachmentId`、`account` |
| `calendar_attachment_replace` | 替换事件附件 | `id`、`attachmentId`、`file`、`account` |
| `calendar_attachment_bulk_delete` | 批量删除事件附件 | `id`、`attachmentIds`、`confirm`、`account` |

说明：`calendar_respond` 只能用于别人发来的会议邀请；如果当前账号是会议组织者，Graph 会拒绝响应。`calendar_cancel` 通常也需要当前账号是会议组织者。

`calendar_attachment_add` 支持最大 150 MiB 的事件附件；超过 3 MiB 时会自动使用 Graph 上传会话。

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
| `contacts_bulk_delete` | 批量删除联系人，默认 dry-run | `ids`、`confirm`、`account` |
| `contacts_duplicates_scan` | 扫描重复联系人 | `strategy`、`folder`、`max`、`account` |
| `contacts_duplicates_merge` | 合并重复联系人，默认 dry-run | `strategy`、`folder`、`max`、`confirm`、`account` |
| `sync_run` | 同步指定账号的 SQLite/FTS 索引，delta 不可用时回退全量 | `mail`、`contacts`、`events`、`max`、`account` |
| `sync_status` | 查看本地索引状态 | `account` |
| `sync_reset` | 删除本地索引 | `account`、`all`、`confirm` |
| `local_search` | 搜索本地 SQLite/FTS 索引 | `query`、`type`、`limit`、`account` |
| `sync_compact` | 压缩和优化 SQLite 索引 | 无 |
| `sync_prune` | 清理旧邮件索引 | `account`、`olderThanDays`、`confirm` |
| `contacts_export` | 导出联系人到 CSV 或 vCard | `file`、`format`、`folder`、`max`、`account` |
| `contacts_import` | 从 CSV 或 vCard 导入联系人 | `file`、`format`、`folder`、`max`、`confirm`、`account` |

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

### 7.11 创建分类规则并立即回放

```json
{
  "tool": "mail_rule_create",
  "arguments": {
    "name": "发票分类",
    "from": ["billing@example.com"],
    "assignCategories": ["财务"],
    "markAsRead": true,
    "disabled": true,
    "applyNow": true,
    "confirmApply": true,
    "applyMax": 500
  }
}
```

不传 `confirmApply: true` 时只会对已有邮件做 dry-run，不会实际修改。

### 7.12 回放已有规则

```json
{
  "tool": "mail_rule_apply",
  "arguments": {
    "id": "<rule-id>",
    "max": 500,
    "confirm": true
  }
}
```

`mail_rule_apply` 默认也是 dry-run；确认后加 `"confirm": true`。

说明：Microsoft Graph 没有「立即运行规则」的官方端点。这里是本地回放功能，只支持可安全解析的常见条件和动作；遇到不支持的规则条件或动作会拒绝执行，避免误处理邮件。

当前回放支持完整正文匹配（HTML 会转换为纯文本）、多文件夹扫描，以及 `copyToFolder`、`forwardTo`、`redirectTo` 等动作。
### 7.13 转发日历事件

```json
{
  "tool": "calendar_forward",
  "arguments": {
    "id": "<event-id>",
    "to": ["someone@example.com"],
    "comment": "请参加"
  }
}
```
### 7.14 删除或替换草稿附件

```json
{
  "tool": "mail_draft_attachment_delete",
  "arguments": {
    "id": "<draft-id>",
    "attachmentId": "<attachment-id>"
  }
}
```

```json
{
  "tool": "mail_draft_attachment_replace",
  "arguments": {
    "id": "<draft-id>",
    "attachmentId": "<old-attachment-id>",
    "file": "C:\\temp\\new.pdf"
  }
}
```

### 7.15 联系人 CSV 导入导出

导出：

```json
{
  "tool": "contacts_export",
  "arguments": {
    "file": "C:\\temp\\contacts.csv",
    "max": 500
  }
}
```

导入默认是 dry-run：

```json
{
  "tool": "contacts_import",
  "arguments": {
    "file": "C:\\temp\\contacts.csv",
    "confirm": false
  }
}
```

确认导入：

```json
{
  "tool": "contacts_import",
  "arguments": {
    "file": "C:\\temp\\contacts.csv",
    "confirm": true
  }
}
```
### 7.16 删除或替换日历事件附件

```json
{
  "tool": "calendar_attachment_delete",
  "arguments": {
    "id": "<event-id>",
    "attachmentId": "<attachment-id>"
  }
}
```

```json
{
  "tool": "calendar_attachment_replace",
  "arguments": {
    "id": "<event-id>",
    "attachmentId": "<old-attachment-id>",
    "file": "C:\\temp\\new.pdf"
  }
}
```

### 7.17 查询审计日志

```json
{
  "tool": "audit_list",
  "arguments": {
    "limit": 20,
    "action": "mail."
  }
}
```
### 7.18 邮件文件夹树与批量操作

```json
{
  "tool": "mail_folder_tree",
  "arguments": {
    "top": 200
  }
}
```

```json
{
  "tool": "mail_bulk_move",
  "arguments": {
    "ids": ["<id1>", "<id2>"],
    "folder": "归档",
    "confirm": true
  }
}
```

```json
{
  "tool": "calendar_attachment_bulk_delete",
  "arguments": {
    "id": "<event-id>",
    "attachmentIds": ["<att1>", "<att2>"],
    "confirm": true
  }
}
```

### 7.19 联系人 vCard 导入导出

```json
{
  "tool": "contacts_export",
  "arguments": {
    "file": "C:\\temp\\contacts.vcf",
    "format": "vcf"
  }
}
```

```json
{
  "tool": "contacts_import",
  "arguments": {
    "file": "C:\\temp\\contacts.vcf",
    "format": "vcf",
    "confirm": true
  }
}
```
### 7.20 邮箱设置

```json
{
  "tool": "mailbox_settings_get",
  "arguments": {}
}
```

```json
{
  "tool": "mailbox_auto_reply_set",
  "arguments": {
    "status": "alwaysEnabled",
    "internalMessage": "我正在休假",
    "externalMessage": "I am out of office"
  }
}
```

### 7.21 高级邮件查询和保存视图

```json
{
  "tool": "mail_query",
  "arguments": {
    "folder": "inbox",
    "from": "noreply@example.com",
    "unreadOnly": true,
    "hasAttachment": true,
    "after": "2026-10-01",
    "top": 100
  }
}
```

```json
{
  "tool": "mail_view_save",
  "arguments": {
    "name": "未读重要",
    "from": "noreply@example.com",
    "unreadOnly": true,
    "top": 100
  }
}
```

```json
{
  "tool": "mail_view_run",
  "arguments": {
    "name": "未读重要"
  }
}
```

### 7.22 批量邮件状态操作

```json
{
  "tool": "mail_bulk_mark_read",
  "arguments": {
    "ids": ["<id1>", "<id2>"],
    "confirm": true
  }
}
```

```json
{
  "tool": "mail_bulk_set_importance",
  "arguments": {
    "ids": ["<id1>", "<id2>"],
    "level": "high",
    "confirm": true
  }
}
```

### 7.23 EML/MIME 导入导出

```json
{
  "tool": "mail_export_eml",
  "arguments": {
    "id": "<message-id>",
    "file": "C:\\temp\\mail.eml"
  }
}
```

```json
{
  "tool": "mail_import_eml",
  "arguments": {
    "file": "C:\\temp\\mail.eml",
    "confirm": true
  }
}
```

### 7.24 日历 ICS、系列和空闲查询

```json
{
  "tool": "calendar_export_ics",
  "arguments": {
    "file": "C:\\temp\\calendar.ics"
  }
}
```

```json
{
  "tool": "calendar_series_update",
  "arguments": {
    "id": "<event-id>",
    "scope": "series",
    "subject": "新的系列主题"
  }
}
```

```json
{
  "tool": "calendar_free_busy",
  "arguments": {
    "schedules": ["someone@example.com"],
    "start": "2026-11-01T00:00:00Z",
    "end": "2026-11-02T00:00:00Z"
  }
}
```

### 7.25 联系人重复合并

```json
{
  "tool": "contacts_duplicates_scan",
  "arguments": {
    "strategy": "email"
  }
}
```

```json
{
  "tool": "contacts_duplicates_merge",
  "arguments": {
    "strategy": "email",
    "confirm": true
  }
}
```

### 7.26 本地索引和离线搜索

```json
{
  "tool": "sync_run",
  "arguments": {
    "mail": true,
    "contacts": true,
    "events": true,
    "max": 1000
  }
}
```

```json
{
  "tool": "local_search",
  "arguments": {
    "query": "账单",
    "type": "all",
    "limit": 20
  }
}
```

> 个人 Microsoft 账号可能不支持部分 Graph delta 端点；此时 `sync_run` 会自动回退为全量同步，并在 `warnings` 中说明。

本地索引使用 SQLite + FTS5，按账号隔离；`sync compact` 可执行 `PRAGMA optimize` 和 `VACUUM`。
邮件 delta 不可用时使用 `lastModifiedDateTime` 高水位增量；`sync prune` 可清理旧邮件索引。

### 7.27 审计日志增强

```json
{
  "tool": "audit_export",
  "arguments": {
    "file": "C:\\temp\\audit.csv",
    "format": "csv",
    "account": "someone@example.com",
    "since": "2026-01-01"
  }
}
```

```json
{
  "tool": "audit_verify",
  "arguments": {}
}
```

```json
{
  "tool": "audit_config_set",
  "arguments": {
    "maxSizeMB": 10,
    "maxBackups": 5,
    "retentionDays": 30
  }
}
```

### 7.28 自动更新检查

```json
{
  "tool": "update_check",
  "arguments": {}
}
```

## 8. MCP tool annotations 和结构化输出

所有 MCP tools 都带有 annotations：

- `readOnlyHint`：是否为只读工具
- `destructiveHint`：是否可能执行删除、永久删除、规则回放等破坏性操作
- `openWorldHint`：是否访问 Microsoft Graph 外部环境
- `idempotentHint`：只读工具标记为幂等

每个 tool 都有独立的 `outputSchema`：

- `$id` 按 tool 名称生成
- 顶层包含 `success`、`account`、`result`、`error`
- 成功结果写入 `structuredContent.result`
- 失败结果写入 `structuredContent.error`
- 失败时会设置 `isError: true`

统一错误码包括：

```text
unauthorized
forbidden
not_found
rate_limited
invalid_argument
timeout
network_error
auth_error
graph_error
```

客户端可据此决定是否要求用户确认，也可以根据错误码自动重试。

## 9. 返回格式

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

## 10. 故障排查

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

## 11. 安全说明

- token-cache.bin 使用系统安全存储加密；Windows 只能由同一用户解密，Linux/macOS 通过系统钥匙串保护密钥
- accounts.json 是可读账号元数据，不包含 token
- 不要把 token-cache.bin 提交到仓库或发送给他人
- `logs/audit.log` 记录关键写操作，不记录邮件正文、附件内容或 token
- 当前审计日志默认单文件上限 10 MiB，保留最近 5 个历史文件，并清理 30 天前的日志。
- MCP tools 可以读取本机指定路径并上传为附件，请只连接可信的 MCP 客户端
- 永久删除、取消会议、发送邮件等操作不可逆，建议在自动化流程中增加人工确认

## 12. 发布

GitHub Actions 在推送 `v*` tag 时构建 Windows amd64 二进制：

```text
outlookctl-windows-amd64.exe
outlook-mcp-windows-amd64.exe
```

发布前需要确认 tag 对应提交已经在 `main` 分支上。