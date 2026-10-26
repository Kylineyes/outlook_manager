// Package update 负责检查 GitHub Releases 最新版本。
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// DefaultAPIBaseURL 是 GitHub API 地址。
const DefaultAPIBaseURL = "https://api.github.com"

// Repository 是发布仓库。
const Repository = "Kylineyes/outlook_manager"

// APIBaseURL 可在测试中覆盖。
var APIBaseURL = DefaultAPIBaseURL

// Asset 表示发布资产。
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// Release 表示 GitHub Release。
type Release struct {
	TagName     string  `json:"tag_name"`
	Name        string  `json:"name"`
	HTMLURL     string  `json:"html_url"`
	PublishedAt string  `json:"published_at"`
	Body        string  `json:"body"`
	Assets      []Asset `json:"assets"`
}

// Result 表示更新检查结果。
type Result struct {
	Current         string   `json:"current"`
	Latest          string   `json:"latest"`
	UpdateAvailable bool     `json:"updateAvailable"`
	IsDevelopment   bool     `json:"isDevelopment"`
	Release         *Release `json:"release,omitempty"`
}

// Check 查询最新 Release 并比较版本。
func Check(ctx context.Context, current string) (*Result, error) {
	endpoint := strings.TrimRight(APIBaseURL, "/") + "/repos/" + Repository + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建更新检查请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "outlook-manager")
	if token := firstEnv("GITHUB_TOKEN", "GH_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 GitHub Releases 失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("仓库还没有发布 Release")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub Releases 返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var release Release
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, fmt.Errorf("解析 GitHub Release 失败: %w", err)
	}
	result := &Result{Current: current, Latest: release.TagName, Release: &release}
	currentNormalized := normalizeVersion(current)
	latestNormalized := normalizeVersion(release.TagName)
	if currentNormalized == "" || latestNormalized == "" {
		result.IsDevelopment = true
		return result, nil
	}
	result.UpdateAvailable = compareVersion(latestNormalized, currentNormalized) > 0
	return result, nil
}

func firstEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func normalizeVersion(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "v")
	if value == "" || value == "dev" {
		return ""
	}
	return value
}

func compareVersion(left, right string) int {
	leftCore, leftPre := splitVersion(left)
	rightCore, rightPre := splitVersion(right)
	max := len(leftCore)
	if len(rightCore) > max {
		max = len(rightCore)
	}
	for i := 0; i < max; i++ {
		l, r := 0, 0
		if i < len(leftCore) {
			l = leftCore[i]
		}
		if i < len(rightCore) {
			r = rightCore[i]
		}
		if l > r {
			return 1
		}
		if l < r {
			return -1
		}
	}
	if leftPre == rightPre {
		return 0
	}
	if leftPre == "" {
		return 1
	}
	if rightPre == "" {
		return -1
	}
	if leftPre > rightPre {
		return 1
	}
	return -1
}

func splitVersion(value string) ([]int, string) {
	core := value
	pre := ""
	if index := strings.IndexAny(core, "-+"); index >= 0 {
		pre = core[index+1:]
		core = core[:index]
	}
	parts := strings.Split(core, ".")
	numbers := make([]int, 0, len(parts))
	for _, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil {
			return []int{0}, value
		}
		numbers = append(numbers, number)
	}
	return numbers, pre
}
