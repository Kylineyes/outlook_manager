param(
    [ValidateSet("quick", "full")]
    [string]$Profile = "quick",
    [string]$Output = "perf-localindex.txt"
)

$ErrorActionPreference = "Stop"
$env:CGO_ENABLED = "0"

if ($Profile -eq "full") {
    $pattern = "BenchmarkSearch10K|BenchmarkSearch100K|BenchmarkSearch500K|BenchmarkMultiAccountSearch|BenchmarkBulkUpsert10K|BenchmarkBulkUpsert100K"
} else {
    $pattern = "BenchmarkSearch10K|BenchmarkSearch100K|BenchmarkMultiAccountSearch|BenchmarkBulkUpsert10K"
}

Write-Host "运行本地索引性能测试: $Profile"
go test ./internal/localindex -run '^$' -bench $pattern -benchtime=1x -benchmem -timeout 600s 2>&1 | Tee-Object -FilePath $Output
Write-Host "结果已写入: $Output"