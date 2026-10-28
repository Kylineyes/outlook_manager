//go:build windows

package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

func replaceBinary(currentPath, newPath, backupPath string) error {
	return replaceBinaryForProcess(currentPath, newPath, backupPath, os.Getpid())
}

func replaceBinaryForProcess(currentPath, newPath, backupPath string, targetPID int) error {
	scriptPath := filepath.Join(os.TempDir(), fmt.Sprintf("outlook-manager-update-%d.ps1", os.Getpid()))
	script := `param([int]$TargetPid, [string]$OldPath, [string]$NewPath, [string]$BackupPath)
while (Get-Process -Id $TargetPid -ErrorAction SilentlyContinue) {
	Start-Sleep -Milliseconds 500
}
if ($BackupPath -ne '') {
	Copy-Item -LiteralPath $OldPath -Destination $BackupPath -Force
}
Remove-Item -LiteralPath $OldPath -Force -ErrorAction SilentlyContinue
Move-Item -LiteralPath $NewPath -Destination $OldPath -Force
Remove-Item -LiteralPath $MyInvocation.MyCommand.Path -Force -ErrorAction SilentlyContinue
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		return fmt.Errorf("创建更新脚本失败: %w", err)
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-File", scriptPath, strconv.Itoa(targetPID), currentPath, newPath, backupPath)
	if err := cmd.Start(); err != nil {
		_ = os.Remove(scriptPath)
		return fmt.Errorf("启动更新脚本失败: %w", err)
	}
	_ = cmd.Process.Release()
	return nil
}
