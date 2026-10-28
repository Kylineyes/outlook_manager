# 本地索引性能测试

本地索引使用 SQLite + FTS5。性能基准位于：

```text
internal/localindex/index_test.go
```

## 运行方式

快速测试：

```text
go test ./internal/localindex -run '^$' -bench 'BenchmarkSearch10K|BenchmarkSearch100K|BenchmarkMultiAccountSearch|BenchmarkBulkUpsert10K' -benchtime=1x -benchmem -timeout 300s
```

完整测试：

```text
go test ./internal/localindex -run '^$' -bench 'BenchmarkSearch10K|BenchmarkSearch100K|BenchmarkSearch500K|BenchmarkMultiAccountSearch|BenchmarkBulkUpsert10K|BenchmarkBulkUpsert100K' -benchtime=1x -benchmem -timeout 600s
```

也可以使用脚本：

```text
scripts\perf-localindex.ps1 -Profile quick
scripts\perf-localindex.ps1 -Profile full

bash scripts/perf-localindex.sh quick
bash scripts/perf-localindex.sh full
```

## 测试数据集

- 每条消息使用唯一 ID
- 主题和正文为中文，覆盖 FTS5 与 LIKE 回退路径
- 搜索固定返回前 20 条
- 每次基准前创建独立临时数据库
- 多账号基准使用 4 个账号，每个 25,000 条消息

## 本机结果

测试环境：

```text
Windows amd64
Intel Core i7-14700K
Go 1.27.2
```

| Benchmark | 数据量 | 搜索耗时 | 数据库大小 |
|---|---:|---:|---:|
| Search10K | 10,000 | 10.9 ms | 16.2 MB |
| Search100K | 100,000 | 137.8 ms | 89.2 MB |
| Search500K | 500,000 | 535.2 ms | 407.3 MB |
| MultiAccountSearch | 4 x 25,000 | 93.9 ms | 100.1 MB |

批量写入：

| Benchmark | 数据量 | 耗时 | 内存分配 |
|---|---:|---:|---:|
| BulkUpsert10K | 10,000 | 510.7 ms | 45.5 MB |
| BulkUpsert100K | 100,000 | 5.17 s | 455.0 MB |

以上为单次基准结果，实际性能会受磁盘、杀毒软件、SQLite 缓存和系统负载影响。

## 索引优化

性能测试过程中做了两项关键优化：

1. FTS 文档映射表 `search_docs`
   - 原实现每次 upsert 都在 FTS 表上按账号、类型、ID 做全表扫描
   - 现在通过 `doc_id` 做 O(log n) 查找，并按 FTS rowid 精确删除
2. SQLite 连接与缓存参数
   - WAL 模式
   - 4 个连接支持并发读取
   - 64 MB page cache
   - 256 MB mmap
   - `wal_autocheckpoint=1000`

`sync compact` 会执行：

```sql
PRAGMA optimize;
PRAGMA wal_checkpoint(TRUNCATE);
VACUUM;
PRAGMA wal_checkpoint(TRUNCATE);
```

## 注意事项

- 500K benchmark 会创建约 400 MB 临时数据库，并消耗较多磁盘和内存。
- 建议在空闲机器上运行完整测试。
- 搜索结果包含 LIKE 回退，因此耗时不仅取决于 FTS。
- benchmark 使用 `-benchtime=1x`，用于观察单次大查询延迟，不是稳定吞吐测试。