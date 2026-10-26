package update

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/store"
)

// DownloadResult 表示一次更新下载和校验结果。
type DownloadResult struct {
	Version           string `json:"version"`
	AssetName         string `json:"assetName"`
	ArchivePath       string `json:"archivePath"`
	BinaryPath        string `json:"binaryPath"`
	ChecksumVerified  bool   `json:"checksumVerified"`
	SignatureVerified bool   `json:"signatureVerified"`
	SignatureWarning  string `json:"signatureWarning,omitempty"`
}

// ApplyResult 表示替换当前二进制的执行结果。
type ApplyResult struct {
	ExePath         string `json:"exePath"`
	BackupPath      string `json:"backupPath,omitempty"`
	RestartRequired bool   `json:"restartRequired"`
	Message         string `json:"message,omitempty"`
}

// AssetPrefixForPlatform 返回当前平台压缩包前缀。
func AssetPrefixForPlatform() (string, error) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "windows/amd64":
		return "outlook-manager-windows-amd64-x64-", nil
	case "windows/arm64":
		return "outlook-manager-windows-arm64-", nil
	case "linux/amd64":
		return "outlook-manager-linux-amd64-x64-", nil
	case "linux/arm64":
		return "outlook-manager-linux-arm64-", nil
	case "darwin/amd64":
		return "outlook-manager-darwin-amd64-", nil
	case "darwin/arm64":
		return "outlook-manager-darwin-arm64-", nil
	default:
		return "", fmt.Errorf("当前平台暂无自动更新包: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

// FindArchiveAsset 查找当前平台压缩包。
func FindArchiveAsset(release *Release) (*Asset, error) {
	if release == nil {
		return nil, fmt.Errorf("Release 不能为空")
	}
	prefix, err := AssetPrefixForPlatform()
	if err != nil {
		return nil, err
	}
	for i := range release.Assets {
		if strings.HasPrefix(release.Assets[i].Name, prefix) && strings.HasSuffix(release.Assets[i].Name, ".zip") {
			return &release.Assets[i], nil
		}
	}
	return nil, fmt.Errorf("Release 中找不到当前平台压缩包: %s*", prefix)
}

func findNamedAsset(release *Release, name string) *Asset {
	for i := range release.Assets {
		if release.Assets[i].Name == name {
			return &release.Assets[i]
		}
	}
	return nil
}

// DownloadRelease 下载并校验当前平台更新包，并解压当前二进制。
func DownloadRelease(ctx context.Context, release *Release, dir string) (*DownloadResult, error) {
	archiveAsset, err := FindArchiveAsset(release)
	if err != nil {
		return nil, err
	}
	checksumsAsset := findNamedAsset(release, "SHA256SUMS")
	if checksumsAsset == nil {
		return nil, fmt.Errorf("Release 缺少 SHA256SUMS")
	}
	if strings.TrimSpace(dir) == "" {
		base, err := store.EnsureBaseDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(base, "updates", release.TagName)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("创建更新目录失败: %w", err)
	}
	archivePath := filepath.Join(dir, archiveAsset.Name)
	checksumsPath := filepath.Join(dir, "SHA256SUMS")
	if err := downloadFile(ctx, archiveAsset.BrowserDownloadURL, archivePath); err != nil {
		return nil, err
	}
	if err := downloadFile(ctx, checksumsAsset.BrowserDownloadURL, checksumsPath); err != nil {
		return nil, err
	}
	expected, err := checksumForFile(checksumsPath, archiveAsset.Name)
	if err != nil {
		return nil, err
	}
	actual, err := fileSHA256(archivePath)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(expected, actual) {
		return nil, fmt.Errorf("SHA256 校验失败: expected=%s actual=%s", expected, actual)
	}
	result := &DownloadResult{
		Version: release.TagName, AssetName: archiveAsset.Name,
		ArchivePath: archivePath, ChecksumVerified: true,
	}
	result.SignatureVerified, result.SignatureWarning = verifyOptionalGPG(ctx, release, dir)
	binaryName := currentBinaryName()
	binaryPath := filepath.Join(dir, binaryName)
	if err := extractZipBinary(archivePath, binaryPath, binaryName); err != nil {
		return nil, err
	}
	result.BinaryPath = binaryPath
	return result, nil
}

func downloadFile(ctx context.Context, source, target string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return fmt.Errorf("创建下载请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "outlook-manager")
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("下载返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("创建下载文件失败: %w", err)
	}
	defer file.Close()
	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("写入下载文件失败: %w", err)
	}
	return nil
}

func checksumForFile(path, filename string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取 SHA256SUMS 失败: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		if filepath.Base(name) == filepath.Base(filename) {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("SHA256SUMS 中找不到 %s", filename)
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("计算 SHA256 失败: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func currentBinaryName() string {
	name := "outlookctl"
	if exe, err := os.Executable(); err == nil {
		base := strings.ToLower(filepath.Base(exe))
		if strings.Contains(base, "outlook-mcp") {
			name = "outlook-mcp"
		}
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

func extractZipBinary(archivePath, targetPath, binaryName string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("打开更新压缩包失败: %w", err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if filepath.Base(file.Name) != binaryName {
			continue
		}
		source, err := file.Open()
		if err != nil {
			return fmt.Errorf("打开压缩包内二进制失败: %w", err)
		}
		defer source.Close()
		target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return fmt.Errorf("创建新二进制失败: %w", err)
		}
		if _, err := io.Copy(target, source); err != nil {
			target.Close()
			return fmt.Errorf("解压二进制失败: %w", err)
		}
		return target.Close()
	}
	return fmt.Errorf("压缩包中找不到二进制: %s", binaryName)
}

func verifyOptionalGPG(ctx context.Context, release *Release, dir string) (bool, string) {
	signatureAsset := findNamedAsset(release, "SHA256SUMS.asc")
	publicKeyAsset := findNamedAsset(release, "public-key.asc")
	if signatureAsset == nil || publicKeyAsset == nil {
		return false, "Release 未提供 GPG 签名"
	}
	signaturePath := filepath.Join(dir, "SHA256SUMS.asc")
	publicKeyPath := filepath.Join(dir, "public-key.asc")
	if err := downloadFile(ctx, signatureAsset.BrowserDownloadURL, signaturePath); err != nil {
		return false, "下载 GPG 签名失败: " + err.Error()
	}
	if err := downloadFile(ctx, publicKeyAsset.BrowserDownloadURL, publicKeyPath); err != nil {
		return false, "下载 GPG 公钥失败: " + err.Error()
	}
	if _, err := exec.LookPath("gpg"); err != nil {
		return false, "系统未安装 gpg，跳过签名校验"
	}
	home, err := os.MkdirTemp("", "outlook-manager-gpg-*")
	if err != nil {
		return false, "创建 GPG 临时目录失败: " + err.Error()
	}
	defer os.RemoveAll(home)
	env := append(os.Environ(), "GNUPGHOME="+home)
	importCmd := exec.CommandContext(ctx, "gpg", "--batch", "--import", publicKeyPath)
	importCmd.Env = env
	if output, err := importCmd.CombinedOutput(); err != nil {
		return false, "导入 GPG 公钥失败: " + strings.TrimSpace(string(output))
	}
	verifyCmd := exec.CommandContext(ctx, "gpg", "--batch", "--verify", signaturePath, filepath.Join(dir, "SHA256SUMS"))
	verifyCmd.Env = env
	if output, err := verifyCmd.CombinedOutput(); err != nil {
		return false, "GPG 签名校验失败: " + strings.TrimSpace(string(output))
	}
	return true, ""
}

// ApplyDownload 备份并替换当前二进制。
func ApplyDownload(result *DownloadResult) (*ApplyResult, error) {
	if result == nil || strings.TrimSpace(result.BinaryPath) == "" {
		return nil, fmt.Errorf("更新文件不能为空")
	}
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("获取当前可执行文件失败: %w", err)
	}
	backupPath := exePath + ".bak"
	if err := replaceBinary(exePath, result.BinaryPath, backupPath); err != nil {
		return nil, err
	}
	apply := &ApplyResult{ExePath: exePath, BackupPath: backupPath}
	if runtime.GOOS == "windows" {
		apply.RestartRequired = true
		apply.Message = "更新将在当前进程退出后完成，请重新启动程序"
	} else {
		apply.RestartRequired = true
		apply.Message = "二进制已替换，请重新启动程序使用新版本"
	}
	return apply, nil
}

// Rollback 使用备份恢复当前二进制。
func Rollback() (*ApplyResult, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("获取当前可执行文件失败: %w", err)
	}
	backupPath := exePath + ".bak"
	if _, err := os.Stat(backupPath); err != nil {
		return nil, fmt.Errorf("找不到更新备份: %s", backupPath)
	}
	if err := replaceBinary(exePath, backupPath, ""); err != nil {
		return nil, err
	}
	result := &ApplyResult{ExePath: exePath, BackupPath: backupPath, RestartRequired: true}
	if runtime.GOOS == "windows" {
		result.Message = "回滚将在当前进程退出后完成，请重新启动程序"
	} else {
		result.Message = "已回滚，请重新启动程序"
	}
	return result, nil
}
