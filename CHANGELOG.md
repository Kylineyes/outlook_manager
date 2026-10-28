# Changelog

本项目使用类似 Keep a Changelog 的格式，版本号遵循语义化版本。

## [Unreleased]

### Added

- Linux Secret Service / macOS Keychain 安全存储
- 自动更新检查：`outlookctl update check` 和 MCP `update_check`
- 审计日志查询、导出、统计和哈希链校验
- SQLite + FTS5 本地索引、多账号隔离和增量事务写入
- 邮件高水位增量同步、索引保留清理和 10 万条 benchmark
- 50 万条搜索、多账号并发、批量 upsert benchmark 和 FTS 映射优化
- 索引压缩优化和 1 万条级搜索 benchmark
- 日历 ICS 导入导出、重复事件系列操作和空闲查询
- 联系人重复检测与合并
- 批量邮件状态操作
- EML/MIME 导入导出
- 多平台发布 workflow、SHA256SUMS 和可选代码签名
- Shell 自动补全、彩色/表格输出和交互菜单
- YAML/JSONL 输出、自定义命令别名、命令建议和统一退出码
- 全屏文本菜单、大附件上传进度和可恢复上传
- CLI 配置文件与输出格式偏好
- 自动更新下载、SHA256/GPG 校验、替换和回滚
- 启动时每日自动更新检查、缓存和 --no-update-check

### Changed

- MCP 工具数量增加到 118
- 测试使用临时 HOME/USERPROFILE 隔离
- ICS golden 测试兼容不同换行符

## [0.1.0] - 2026-10-26

### Added

- Microsoft Graph Device Code 登录
- 多账号管理与 DPAPI token 持久化
- 邮件读取、搜索、发送、回复、转发、草稿和附件
- 邮件文件夹树、规则、类别和批量操作
- 日历事件、附件、邀请响应、ICS 导入导出
- 联系人、联系人文件夹、CSV/vCard 导入导出和照片
- 邮箱设置、高级查询和保存视图
- MCP 结构化输出、工具 annotations 和统一错误码
- 本地审计日志、轮转和查询
- Windows/Linux amd64 Release 压缩包
