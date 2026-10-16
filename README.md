# Outlook Manager

Outlook Manager 是一个通过 Microsoft Graph 官方 API 操作 Outlook 账号的本地工具，同时提供 CLI 和 MCP 两种入口。

## 功能

当前已实现：

- Device Code 登录与 token 加密持久化
- 多账号账号档案与默认账号切换
- 收件箱邮件列表
- 单封邮件详情
- 邮件搜索
- 发送邮件
- 回复、回复全部、转发
- 标记已读 / 未读
- 设置 / 取消旗标
- 设置重要性
- 邮件文件夹列表
- 邮件文件夹创建、重命名、删除
- 邮件移动
- 邮件删除（移动到已删除邮件）
- 邮件附件列表
- 邮件附件下载
- 邮件类别查看、添加、移除
- 日历事件读取
- 日历事件创建、更新、删除
- 日历参与人与提醒
- 联系人读取
- 联系人创建、更新、删除
- 联系人搜索
- 联系人文件夹管理
- 草稿创建、列表、发送
- 邮件列表、搜索、详情、附件对应的 MCP tools

## 环境要求

- Windows
- Go 1.27 或更高版本

## 构建

```powershell
go build -o outlookctl.exe ./cmd/outlookctl
go build -o outlook-mcp.exe ./cmd/outlook-mcp
```

Windows amd64 交叉编译：

```powershell
$env:GOOS='windows'
$env:GOARCH='amd64'
$env:CGO_ENABLED='0'
go build -trimpath -ldflags '-s -w' -o outlookctl.exe ./cmd/outlookctl
go build -trimpath -ldflags '-s -w' -o outlook-mcp.exe ./cmd/outlook-mcp
```

## CLI 使用

登录：

```powershell
outlookctl auth login
```

查看账号与 token 状态：

```powershell
outlookctl auth status
outlookctl account list
outlookctl account use zero-67@hotmail.com
```

邮件操作：

```powershell
outlookctl mail list --top 10
outlookctl mail read <message-id>
outlookctl mail search --query "关键字"
outlookctl mail send --to someone@example.com --subject "主题" --body "正文"
outlookctl mail mark-read <message-id>
outlookctl mail mark-unread <message-id>
outlookctl mail folders
outlookctl mail move <message-id> --folder 存档
outlookctl mail delete <message-id>
outlookctl mail attachments <message-id>
outlookctl mail attachment-save <message-id> --attachment <attachment-id> --out C:\temp\file.pdf
outlookctl mail message-categories <message-id>
outlookctl mail category-add <message-id> --category "名称"
outlookctl mail category-remove <message-id> --category "名称"
outlookctl calendar list --days 7
outlookctl calendar create --subject "主题" --start "2026-10-20T10:00:00" --end "2026-10-20T11:00:00"
outlookctl calendar create --subject "会议" --start "2026-10-20T10:00:00" --end "2026-10-20T11:00:00" --attendees someone@example.com --reminder-minutes 10
outlookctl calendar update --id <事件ID> --subject "主题" --start "2026-10-20T10:00:00" --end "2026-10-20T11:00:00"
outlookctl calendar delete --id <事件ID>
outlookctl contacts list --top 20
outlookctl contacts list --folder "客户" --top 20
outlookctl contacts search --query "张"
outlookctl contacts folders
outlookctl contacts folder-create --name "客户"
outlookctl contacts folder-delete --id <文件夹ID>
outlookctl contacts create --display-name "名称" --email someone@example.com
outlookctl contacts update --id <联系人ID> --display-name "新名称"
outlookctl contacts delete --id <联系人ID>
outlookctl mail reply <message-id> --body "回复内容"
outlookctl mail reply-all <message-id> --body "回复内容"
outlookctl mail forward <message-id> --to someone@example.com --body "转发内容"
outlookctl mail draft-create --to someone@example.com --subject "主题" --body "正文"
outlookctl mail drafts
outlookctl mail draft-send <draft-id>
outlookctl mail folder-create --name "新文件夹"
outlookctl mail folder-rename <folder-id> --name "新名称"
outlookctl mail folder-delete <folder-id>
outlookctl mail flag <message-id>
outlookctl mail unflag <message-id>
outlookctl mail importance <message-id> --level high
```

## MCP

`outlook-mcp` 通过 stdio 提供以下 tools：

- `account_list`
- `auth_status`
- `mail_list`
- `mail_read`
- `mail_search`
- `mail_send`
- `mail_mark_read`
- `mail_mark_unread`
- `mail_folders_list`
- `mail_move`
- `mail_delete`
- `mail_attachments_list`
- `mail_attachment_save`
- `mail_message_categories`
- `mail_category_add`
- `mail_category_remove`
- `calendar_list`
- `calendar_create`
- `calendar_update`
- `calendar_delete`
- `contacts_list`
- `contacts_create`
- `contacts_update`
- `contacts_delete`
- `contacts_search`
- `contacts_folders_list`
- `contacts_folder_create`
- `contacts_folder_delete`
- `mail_reply`
- `mail_reply_all`
- `mail_forward`
- `mail_draft_create`
- `mail_drafts_list`
- `mail_draft_send`
- `mail_folder_create`
- `mail_folder_rename`
- `mail_folder_delete`
- `mail_flag`
- `mail_unflag`
- `mail_set_importance`

## 本地数据

默认数据目录：

```text
%USERPROFILE%\.outlook-manager\
```

文件说明：

- `config.json`：公开认证配置，不保存秘密
- `accounts.json`：可读账号档案，不保存 token
- `token-cache.bin`：DPAPI 加密后的 access token 和 refresh token
- `logs\auth.log`：预留的认证日志位置

## 开发

提交规范：

- Conventional Commits
- 描述使用简体中文

分支流程：

- `feat/*`、`fix/*`、`docs/*`、`test/*` 从 `dev` 创建
- 测试通过后合入 `dev`
- `main` 只接受用户将 `dev` 合并进来

提交前检查：

```powershell
gofmt -w <changed-go-files>
go vet ./...
go test ./...
```

更多工程约定见 `AGENTS.md`。