# Outlook Manager 路线图

本文件记录当前已完成的关键能力，以及后续仍然可以继续开发的事项。用于后续回顾和排期。

## 已完成的关键方向

- Device Code 登录、多账号、跨平台安全存储 token 持久化（DPAPI / Secret Service / Keychain）
- 邮件读取、搜索、发送、回复、转发、草稿、附件、文件夹、规则、主类别
- 批量邮件移动、删除、已读/未读、旗标、重要性、类别操作
- 邮件 EML/MIME 导出，以及 EML 导入为草稿
- 日历事件、日历列表、附件、邀请响应、事件转发
- 日历 ICS 导入导出
- 重复日历事件系列信息与单次/系列更新删除
- 日历空闲/忙碌查询与共同会议时间查询
- 联系人、联系人文件夹、地址、照片、CSV / vCard 导入导出
- 联系人重复检测与合并
- 本地邮件、联系人、日历索引
- Graph delta 增量同步，失败时自动回退全量同步
- 本地离线搜索
- SQLite + FTS5 本地索引、多账号隔离和增量事务写入
- 邮件高水位增量同步、索引保留清理和大邮箱 benchmark
- 索引压缩优化和 1 万条级搜索 benchmark
- 审计日志轮转、查询、CSV/JSON 导出、账号/时间/动作过滤
- 审计统计、哈希链校验、保留策略配置和手动清理
- CLI 子命令帮助体系
- MCP annotations、结构化输出、独立 outputSchema 和统一错误码
- 邮箱设置管理（自动回复、时区、语言、工作时间）
- 高级邮件查询和保存视图
- mock Graph 集成测试、golden 测试、解析器 fuzz 测试
- 并发/竞态测试、benchmark 和 CI 覆盖率流程
- MCP / CLI 文档和诊断命令
- 多架构发布：Windows/Linux amd64 与 arm64
- SHA256SUMS 与可选 Authenticode/GPG 代码签名
- CHANGELOG、Release Notes 模板和发布文档
- 自动更新检查
- Release 附带 README、LICENSE、CHANGELOG
- Shell 自动补全、彩色/表格输出和交互菜单
- CLI 配置文件与输出格式偏好
- 自动更新下载、校验、替换和回滚

## 待开发事项

剩余事项主要是发布工程和更深的交互体验。

### 1. CLI 体验增强

- 进度条
- 大附件上传进度
- 可恢复上传
- 更好的错误提示
- --output yaml
- 命令别名

### 2. 发布与分发

- Scoop 安装包
- Winget 包
- 安装脚本
- CHANGELOG 自动生成
- 自动下载并应用更新

## 需要第二个测试账号

以下能力需要第二个 Microsoft 账号或共享权限：

- 共享日历
- 委托邮箱
- 代理收发邮件
- 代理日历
- 代理联系人
- 邀请响应全链路验证

## 已知限制

- 个人 Microsoft 账号可能不支持部分 Graph delta 端点。`sync_run` 会检测失败并自动回退全量同步，同时在 `warnings` 中说明。
- Graph 不支持 Outlook contact photo 删除。
- 个人账号在线会议当前可能返回 `onlineMeetingProvider: unknown`。
- Graph 没有官方 run rules now 端点，当前使用本地回放实现。
- 审计账号过滤对升级前没有 `account` 字段的旧记录不生效。
- 审计哈希链只覆盖升级后的新记录，旧记录会在 `audit verify` 中计为 legacy。
