# Outlook Manager

Outlook Manager 是一个通过 Microsoft Graph 官方 API 操作 Outlook 账号的本地工具，同时提供 CLI 和 MCP 两种入口。

## 功能

当前已实现：

- Device Code 登录与 token 加密持久化
- 跨平台安全存储（Windows DPAPI / Linux Secret Service / macOS Keychain）
- Linux 需要可用的 Secret Service（GNOME Keyring、KWallet 等）
- 多账号账号档案与默认账号切换
- 收件箱邮件列表
- 单封邮件详情
- 邮件搜索
- 批量邮件状态操作（已读、旗标、重要性、类别）
- 高级邮件查询
- 保存邮件视图
- 邮箱设置管理（自动回复、时区、语言、工作时间）
- 邮件分页全量拉取
- Graph 与 token 请求临时错误自动重试
- CLI 子命令 --help 体系
- 本地操作审计日志（轮转、查询、导出、统计与哈希链校验）
- 审计保留策略配置与手动清理
- 本地索引、增量同步与离线搜索
- 发送邮件
- 回复、回复全部、转发
- 标记已读 / 未读
- 设置 / 取消旗标
- 设置重要性
- 邮件文件夹列表
- 邮件文件夹创建、重命名、删除
- 邮件文件夹树形展示
- 邮件移动
- 邮件删除（移动到已删除邮件）
- 邮件永久删除
- 邮件附件列表
- 邮件附件下载
- EML/MIME 邮件导出与导入为草稿
- 邮件发送附件
- 邮件大附件上传（3 MiB - 150 MiB，自动使用上传会话）
- 邮件发送选项（重要性、已读回执、送达回执）
- 邮件类别查看、添加、移除
- 收件箱规则管理
- 收件箱规则更新
- 收件箱规则回放（完整正文、多文件夹、dry-run / confirm）
- 规则高级条件与动作（正文、发件人、收件人、分类、复制、移动、转发、重定向、删除）
- 主类别管理
- 主类别颜色更新
- 日历事件读取
- 日历事件 ICS 导入导出
- 重复日历事件系列信息与单次/系列更新删除
- 日历空闲/忙碌与会议时间查询
- 多日历列表与按日历查询事件
- 日历事件创建、更新、删除
- 日历参与人与提醒
- 日历事件分类、状态、重要性、敏感度
- 日历事件附件读取、添加、下载、删除、替换
- 日历事件大附件上传（3 MiB - 150 MiB）
- 日历事件取消
- 日历邀请响应（接受、拒绝、暂定）
- 日历事件转发
- 联系人读取
- 联系人创建、更新、删除
- 联系人搜索
- 联系人重复检测与合并
- 日历重复事件
- 联系人文件夹管理
- 联系人文件夹重命名
- 在联系人文件夹中创建联系人
- 联系人移动到文件夹
- 联系人复制
- 联系人扩展资料（昵称、生日、备注、部门、办公室、职业、主页）
- 联系人地址（家庭、工作、其他）
- 联系人永久删除
- 联系人照片读取与上传
- 草稿创建、列表、发送
- 草稿更新
- 草稿附件删除/替换
- 批量邮件、联系人、附件删除/移动操作
- 联系人批量导入导出（CSV / vCard）
- MCP tool annotations（只读/破坏性提示）
- MCP 结构化输出（outputSchema / structuredContent / 统一错误码）
- MCP 每个 tool 独立 schema 和结构化错误结果
- mock Graph 集成测试、golden 测试和解析器 fuzz 测试
- 并发/竞态测试、benchmark 与 CI 覆盖率流程
- CLI doctor 环境与账号诊断
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
outlookctl mail list --all --max 500
outlookctl mail read <message-id>
outlookctl mail search --query "关键字"
outlookctl mail query --from someone@example.com --unread --has-attachment --after 2026-10-01
outlookctl mail view save "未读重要" --from someone@example.com --unread
outlookctl mail view list
outlookctl mail view run "未读重要"
outlookctl mailbox settings
outlookctl mailbox auto-reply get
outlookctl mailbox timezone get
outlookctl mail send --to someone@example.com --subject "主题" --body "正文"
outlookctl mail send --to someone@example.com --subject "主题" --body "正文" --attach C:\temp\file.pdf
outlookctl mail send --to someone@example.com --subject "主题" --body "正文" --importance high --read-receipt --delivery-receipt
outlookctl mail mark-read <message-id>
outlookctl mail mark-unread <message-id>
outlookctl mail folders
outlookctl mail move <message-id> --folder 存档
outlookctl mail delete <message-id>
outlookctl mail permanent-delete --id <邮件ID>
outlookctl mail attachments <message-id>
outlookctl mail attachment-save <message-id> --attachment <attachment-id> --out C:\temp\file.pdf
outlookctl mail message-categories <message-id>
outlookctl mail category-add <message-id> --category "名称"
outlookctl mail category-remove <message-id> --category "名称"
outlookctl mail rules
outlookctl mail rule-create --name "规则" --from someone@example.com --move-to "存档"
outlookctl mail rule-delete --id <规则ID>
outlookctl mail rule-update --id <规则ID> --name "规则" --subject-contains "关键字" --mark-read
outlookctl mail rule-apply --id <规则ID> [--confirm]
outlookctl mail rule-create --name "分类规则" --subject-contains "关键字" --category "项目A" --apply-now --confirm-apply
outlookctl mail rule-create --name "高级规则" --body-contains "关键字" --has-attachments --copy-to "归档" --forward-to someone@example.com --disabled
outlookctl mail master-categories
outlookctl mail master-category-create --name "项目A" --color preset0
outlookctl mail master-category-delete --id <类别ID>
outlookctl mail master-category-update --id <类别ID> --color preset5
outlookctl calendar list --days 7
outlookctl calendar calendars
outlookctl calendar list --calendar "日历" --days 7
outlookctl calendar attachments --id <事件ID>
outlookctl calendar attachment-add --id <事件ID> --file C:\temp\file.pdf
outlookctl calendar attachment-get --id <事件ID> --attachment-id <附件ID> --out C:\temp\file.pdf
outlookctl calendar attachment-delete --id <事件ID> --attachment-id <附件ID>
outlookctl calendar attachment-replace --id <事件ID> --attachment-id <附件ID> --file C:\temp\new.pdf
outlookctl calendar create --subject "主题" --start "2026-10-20T10:00:00" --end "2026-10-20T11:00:00"
outlookctl calendar create --subject "会议" --start "2026-10-20T10:00:00" --end "2026-10-20T11:00:00" --attendees someone@example.com --reminder-minutes 10
outlookctl calendar create --subject "会议" --start "2026-10-20T10:00:00" --end "2026-10-20T11:00:00" --categories "项目A,重要" --show-as busy --importance high --sensitivity private
outlookctl calendar create --subject "每日站会" --start "2026-10-20T09:00:00" --end "2026-10-20T09:30:00" --repeat daily --repeat-count 5
outlookctl calendar update --id <事件ID> --subject "主题" --start "2026-10-20T10:00:00" --end "2026-10-20T11:00:00"
outlookctl calendar delete --id <事件ID>
outlookctl calendar cancel --id <事件ID> --comment "取消说明"
outlookctl calendar respond --id <事件ID> --response accept
outlookctl calendar forward --id <事件ID> --to someone@example.com
outlookctl calendar export-ics --out C:\temp\calendar.ics
outlookctl calendar import-ics --file C:\temp\calendar.ics --confirm
outlookctl calendar series-info --id <事件ID>
outlookctl calendar series-update --id <事件ID> --scope series --subject "新主题"
outlookctl calendar series-delete --id <事件ID> --scope single
outlookctl calendar freebusy --schedules someone@example.com --start 2026-11-01T00:00:00Z --end 2026-11-02T00:00:00Z
outlookctl calendar find-time --attendees someone@example.com --start 2026-11-01T00:00:00Z --end 2026-11-02T00:00:00Z --duration 60
outlookctl contacts list --top 20
outlookctl contacts list --folder "客户" --top 20
outlookctl contacts search --query "张"
outlookctl contacts folders
outlookctl contacts folder-create --name "客户"
outlookctl contacts folder-rename --id <文件夹ID> --name "新名称"
outlookctl contacts folder-delete --id <文件夹ID>
outlookctl contacts create --display-name "名称" --email someone@example.com
outlookctl contacts create --display-name "名称" --email someone@example.com --nickname "昵称" --birthday "1990-01-02" --department "研发部" --office "A栋101" --profession "工程师" --website "https://example.com"
outlookctl contacts create --folder "客户" --display-name "名称" --email someone@example.com
outlookctl contacts move --id <联系人ID> --folder "客户"
outlookctl contacts copy --id <联系人ID> --folder "客户" --name "副本名称"
outlookctl contacts export --out C:\temp\contacts.csv
outlookctl contacts import --file C:\temp\contacts.csv --confirm
outlookctl contacts export --out C:\temp\contacts.vcf --format vcf
outlookctl contacts import --file C:\temp\contacts.vcf --format vcf --confirm
outlookctl contacts duplicates scan --strategy email
outlookctl contacts duplicates merge --strategy email --confirm
outlookctl contacts update --id <联系人ID> --display-name "新名称"
outlookctl contacts delete --id <联系人ID>
outlookctl contacts permanent-delete --id <联系人ID>
outlookctl contacts photo-set --id <联系人ID> --file C:\temp\photo.jpg
outlookctl contacts photo-get --id <联系人ID> --out C:\temp\photo.jpg
outlookctl contacts create --display-name "名称" --email someone@example.com --home-address "家庭路1号|上海|上海|200000|中国"
outlookctl mail reply <message-id> --body "回复内容"
outlookctl mail reply-all <message-id> --body "回复内容"
outlookctl mail forward <message-id> --to someone@example.com --body "转发内容"
outlookctl mail draft-create --to someone@example.com --subject "主题" --body "正文"
outlookctl mail draft-create --to someone@example.com --subject "主题" --body "正文" --attach C:\temp\file.pdf
outlookctl mail drafts
outlookctl mail draft-send <draft-id>
outlookctl mail draft-update --id <草稿ID> --to someone@example.com --subject "新主题" --body "新正文"
outlookctl mail draft-attachment-delete --id <草稿ID> --attachment-id <附件ID>
outlookctl mail draft-attachment-replace --id <草稿ID> --attachment-id <附件ID> --file C:\temp\new.pdf
outlookctl mail folder-create --name "新文件夹"
outlookctl mail folder-rename <folder-id> --name "新名称"
outlookctl mail folder-delete <folder-id>
outlookctl mail flag <message-id>
outlookctl mail unflag <message-id>
outlookctl mail importance <message-id> --level high
```

## MCP

`outlook-mcp` 通过 stdio 暴露 116 个 MCP tools，覆盖：

- 账号与认证状态
- 邮件读取、搜索、发送、回复、转发和草稿
- 邮件附件、文件夹、类别、旗标和重要性
- 收件箱规则与主类别
- 日历事件、日历列表、事件附件和邀请响应
- 日历 ICS 导入导出、重复事件系列和空闲查询
- 联系人重复检测与合并
- 本地索引、增量同步和离线搜索
- 联系人、联系人文件夹、地址和照片

完整的 MCP 客户端配置、工具参数、调用示例和故障排查见：

```text
docs/mcp.md
```

发布签名和校验说明见 `docs/release-signing.md`，版本变更见 `CHANGELOG.md`。

双账号日历验证计划见：`docs/calendar-invite-test-plan.md`。

后续开发计划见 `ROADMAP.md`。

## 本地数据

默认数据目录：

```text
%USERPROFILE%\.outlook-manager\
```

文件说明：

- `config.json`：公开认证配置，不保存秘密
- `accounts.json`：可读账号档案，不保存 token
- `token-cache.bin`：系统安全存储加密后的 access token 和 refresh token
- `logs\auth.log`：预留的认证日志位置
- `logs\audit.log`：JSON Lines 操作审计日志
- `views.json`：保存的邮件查询视图
- `index.json`：邮件、联系人和日历的本地索引
- `audit-config.json`：审计日志轮转和保留策略

查询审计日志：

```text
outlookctl audit list --limit 20
outlookctl doctor
outlookctl mail folder-tree
outlookctl mail bulk-delete --ids <id1,id2> [--confirm]
outlookctl mail bulk-mark-read --ids <id1,id2> [--confirm]
outlookctl mail bulk-flag --ids <id1,id2> [--confirm]
outlookctl mail bulk-importance --ids <id1,id2> --level high [--confirm]
outlookctl mail bulk-category-add --ids <id1,id2> --category <名称> [--confirm]
outlookctl mail export-eml <邮件ID> --out C:\temp\mail.eml
outlookctl mail import-eml --file C:\temp\mail.eml --confirm
outlookctl contacts bulk-delete --ids <id1,id2> [--confirm]
outlookctl sync run
outlookctl sync status
outlookctl sync reset --confirm
outlookctl search --query <关键字> --type all
outlookctl audit list --action mail. --json
outlookctl audit export --out C:\temp\audit.csv --format csv
outlookctl audit stats --json
outlookctl audit verify --json
outlookctl audit config set --max-size-mb 10 --max-backups 5 --retention-days 30
outlookctl audit cleanup
outlookctl update check
```

如果 GitHub API 限流或仓库为私有，可设置 `GITHUB_TOKEN` 或 `GH_TOKEN`。

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