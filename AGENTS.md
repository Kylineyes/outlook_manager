# AGENTS.md

本项目用于约束和指导参与本仓库工作的 AI Agent。任何 Agent 在修改代码、文档、CI 或提交记录前，都应先阅读并遵守本文件。

## 1. 项目背景

Outlook Manager 是一个本地工具，通过 Microsoft Graph 官方 API 操作 Outlook 账号，并同时提供 CLI 和 MCP 两种入口。目标是让个人 Outlook.com 账号能够被安全、可控地管理和自动化操作。

## 2. 项目目标

- 只使用 Microsoft 官方 API 和官方 SDK。
- 支持个人 Outlook.com 多账号登录、切换和本地加密持久化。
- 提供 CLI 与 MCP 双入口，并共享同一套核心逻辑。
- 第一阶段覆盖邮件能力，后续扩展到日历、联系人等能力。
- 在 main 分支打 tag 后，自动发布 Windows amd64（64 位）可执行文件。

## 3. 当前工程建设构想

### 3.1 技术栈

- 语言：Go。
- 认证：直接调用 Microsoft 身份平台 OAuth2 Device Code Flow；不使用 MSAL Go，因为其对个人账号的 `offline_access` 存在已知误判。
- Graph：`github.com/microsoftgraph/msgraph-sdk-go`，必要时直接调用 Microsoft Graph REST API。
- MCP：`github.com/modelcontextprotocol/go-sdk`。
- 本地加密：Windows 下使用 DPAPI，保护 token cache。
- 模块路径：`github.com/Kylineyes/outlook-manager`。

### 3.2 目录结构

```text
outlook_manager/
├── cmd/
│   ├── outlookctl/          # CLI 入口
│   └── outlook-mcp/         # MCP server 入口
├── internal/
│   ├── auth/                # 登录、多账号、token 刷新
│   ├── graph/               # Microsoft Graph 业务封装
│   ├── store/               # 本地配置与账号档案
│   └── secure/              # DPAPI 等安全存储
├── .github/workflows/       # GitHub Actions
├── go.mod
├── AGENTS.md
└── README.md
```

### 3.3 本地数据

默认数据目录为 `%USERPROFILE%\.outlook-manager\`：

- `config.json`：公开配置，例如 clientId、authority、scopes，不保存密钥。
- `accounts.json`：账号档案，可读，便于人工确认，不保存 token。
- `token-cache.bin`：MSAL token cache，Windows 下使用 DPAPI 当前用户加密。
- `logs\auth.log`：登录、刷新、失败日志，不记录 token。

### 3.4 第一阶段能力

- `outlookctl auth login`
- `outlookctl auth status`
- `outlookctl account list`
- `outlookctl account use <email>`
- `outlookctl mail list`
- `outlookctl mail read`
- `outlookctl mail search`
- `outlookctl mail send`
- `outlookctl mail mark-read`
- `outlookctl mail mark-unread`
- `outlookctl mail folders`
- `outlookctl mail move`
- `outlookctl mail delete`
- `outlookctl mail permanent-delete`
- `outlookctl mail attachments`
- `outlookctl mail attachment-save`
- `outlookctl mail reply`
- `outlookctl mail reply-all`
- `outlookctl mail forward`
- `outlookctl mail draft-create`
- `outlookctl mail drafts`
- `outlookctl mail draft-send`
- `outlookctl mail folder-create`
- `outlookctl mail folder-rename`
- `outlookctl mail folder-delete`
- `outlookctl mail flag`
- `outlookctl mail unflag`
- `outlookctl mail importance`
- `outlookctl mail message-categories`
- `outlookctl mail category-add`
- `outlookctl mail category-remove`
- `outlookctl mail rules`
- `outlookctl mail rule-create`
- `outlookctl mail rule-delete`
- `outlookctl mail master-categories`
- `outlookctl mail master-category-create`
- `outlookctl mail master-category-delete`
- `outlookctl calendar list`
- `outlookctl calendar calendars`
- `outlookctl calendar create`
- `outlookctl calendar update`
- `outlookctl calendar delete`
- `outlookctl contacts list`
- `outlookctl contacts search`
- `outlookctl contacts folders`
- `outlookctl contacts folder-create`
- `outlookctl contacts folder-rename`
- `outlookctl contacts folder-delete`
- `outlookctl contacts move`
- `outlookctl contacts copy`
- `outlookctl contacts create`
- `outlookctl contacts update`
- `outlookctl contacts delete`
- 对应的 MCP 工具：`account_list`、`auth_status`、`mail_list`、`mail_read`、`mail_search`、`mail_send`、`mail_mark_read`、`mail_mark_unread`、`mail_folders_list`、`mail_move`、`mail_delete`、`mail_permanent_delete`、`mail_attachments_list`、`mail_attachment_save`、`mail_reply`、`mail_reply_all`、`mail_forward`、`mail_draft_create`、`mail_drafts_list`、`mail_draft_send`、`mail_folder_create`、`mail_folder_rename`、`mail_folder_delete`、`mail_flag`、`mail_unflag`、`mail_set_importance`、`mail_message_categories`、`mail_category_add`、`mail_category_remove`、`mail_rules_list`、`mail_rule_create`、`mail_rule_delete`、`mail_master_categories_list`、`mail_master_category_create`、`mail_master_category_delete`、`calendar_list`、`calendar_calendars_list`、`contacts_list`、`calendar_create`、`calendar_update`、`calendar_delete`、`contacts_create`、`contacts_update`、`contacts_delete`、`contacts_search`、`contacts_folders_list`、`contacts_folder_create`、`contacts_folder_delete`、`contacts_folder_rename`、`contacts_move`、`contacts_copy`

## 4. 代码编写准则

- 所有注释、文档字符串和错误提示优先使用简体中文。
- 注释要详尽：导出标识符必须有说明；复杂流程、边界条件、错误恢复、安全决策必须解释原因。
- 代码保持简单直接，避免过度抽象。
- 优先使用 Go 标准库，新增依赖必须能说明必要性。
- 错误必须被处理并附加上下文，使用 `%w` 包装底层错误。
- 敏感信息不得写入代码、日志、测试数据或提交记录。
- Windows 专属代码使用构建标签，例如 `//go:build windows`。
- 提交前必须通过：`gofmt`、`go vet ./...`、`go test ./...`。
- 不允许为了通过测试而删除有效校验或降低安全边界。

## 5. Git 提交规范

统一使用约定式提交（Conventional Commits），提交描述使用简体中文。

常用类型：

- `feat`：新增功能
- `fix`：修复缺陷
- `docs`：文档变更
- `style`：格式调整，不改变逻辑
- `refactor`：重构
- `test`：测试相关
- `chore`：工程杂项
- `ci`：CI/CD 变更

示例：

```text
feat: 新增 Outlook 账号 Device Code 登录
fix: 修复 token 缓存解密失败的问题
docs: 补充 AGENTS.md 项目说明
ci: 添加 main 标签触发 Windows amd64 构建
```

## 6. 分支与合并流程

- `main`：稳定分支，只接受用户本人将 `dev` 合并进来。
- `dev`：集成分支，用于收集已完成并测试通过的功能。
- `feat/<名称>`：每个功能单独开分支开发，分支名使用简短英文或拼音。
- 修复类工作可使用 `fix/<名称>`。

开发流程：

1. 从最新的 `dev` 或 `main` 创建功能分支。
2. 在功能分支完成开发，并补充测试。
3. 本地测试通过后，将功能分支合入 `dev`。
4. 远端 `dev` 由用户本人确认后合并进 `main`。
5. 除非用户明确要求，Agent 不得直接向 `main` 提交或合并代码。

## 7. GitHub Actions 发布规则

- 工作流文件位于 `.github/workflows/release-windows-amd64.yml`。
- 触发条件：在 `main` 分支上打 `v*` 标签并推送。
- 工作流会先校验标签提交是否属于 `main` 分支。
- 构建目标：`GOOS=windows`、`GOARCH=amd64`、`CGO_ENABLED=0`。
- 默认构建两个可执行文件：
  - `outlookctl-windows-amd64.exe`
  - `outlook-mcp-windows-amd64.exe`
- 构建完成后自动创建或更新 GitHub Release，并上传可执行文件。