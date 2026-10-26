# Outlook Manager 路线图

本文件记录当前已完成的关键能力，以及后续仍然可以继续开发的事项。用于后续回顾和排期。

## 已完成的关键方向

- Device Code 登录、多账号、DPAPI token 持久化
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
- 审计日志、轮转和查询
- CLI 子命令帮助体系
- MCP annotations、结构化输出、独立 outputSchema 和统一错误码
- 邮箱设置管理（自动回复、时区、语言、工作时间）
- 高级邮件查询和保存视图
- MCP / CLI 文档和诊断命令

## 待开发事项

剩余事项主要是工程化、体验和发布能力。

### 1. CLI 体验增强

- Shell 自动补全
- 彩色输出
- 表格输出
- 交互式 TUI
- 进度条
- 大附件上传进度
- 可恢复上传
- 配置文件命令
- 更好的错误提示
- --output table|json|yaml
- 命令别名

### 2. 审计日志增强

- 导出 CSV / JSON
- 按账号过滤
- 按时间范围过滤
- 按动作类型过滤
- 统计报告
- 日志签名
- 审计清理策略配置
- 审计查看 TUI

### 3. 测试与质量工程

- mock Graph 集成测试
- Graph 响应 golden files
- CSV / vCard / EML / ICS fuzz 测试
- 大附件上传断点续传测试
- MCP tool contract tests
- 并发测试
- race 测试
- 性能 benchmark
- 覆盖率报告
- CI 上强制测试

### 4. 发布与分发

- CI 自动执行 go vet / go test
- CHANGELOG 自动生成
- Release Notes 模板
- Scoop 安装包
- Winget 包
- 安装脚本
- 代码签名
- arm64 构建
- 版本信息嵌入
- 自动更新检查

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
