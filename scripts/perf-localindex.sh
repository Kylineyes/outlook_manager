#!/usr/bin/env bash
set -euo pipefail

PROFILE="${1:-quick}"
OUTPUT="${2:-perf-localindex.txt}"

if [[ "$PROFILE" == "full" ]]; then
  PATTERN='BenchmarkSearch10K|BenchmarkSearch100K|BenchmarkSearch500K|BenchmarkMultiAccountSearch|BenchmarkBulkUpsert10K|BenchmarkBulkUpsert100K'
else
  PATTERN='BenchmarkSearch10K|BenchmarkSearch100K|BenchmarkMultiAccountSearch|BenchmarkBulkUpsert10K'
fi

export CGO_ENABLED=0
echo "运行本地索引性能测试: $PROFILE"
go test ./internal/localindex -run '^$' -bench "$PATTERN" -benchtime=1x -benchmem -timeout 600s 2>&1 | tee "$OUTPUT"
echo "结果已写入: $OUTPUT"