package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Category 表示 Outlook 主类别列表中的一项。
type Category struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Color       string `json:"color"`
}

// ListMasterCategories 读取当前账号的主类别列表。
func ListMasterCategories(ctx context.Context, accessToken string) ([]Category, error) {
	endpoint := graphBaseURL + "/me/outlook/masterCategories"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建类别列表请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 类别列表失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 类别列表返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		Value []Category `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 Graph 类别列表失败: %w", err)
	}
	return result.Value, nil
}

// GetMessageCategories 读取单封邮件的类别列表。
func GetMessageCategories(ctx context.Context, accessToken, messageID string) ([]string, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil, fmt.Errorf("邮件 ID 不能为空")
	}

	query := url.Values{}
	query.Set("$select", "categories")
	endpoint := graphBaseURL + "/me/messages/" + url.PathEscape(messageID) + "?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建邮件类别请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 邮件类别失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 邮件类别返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		Categories []string `json:"categories"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 Graph 邮件类别失败: %w", err)
	}
	return result.Categories, nil
}

// SetMessageCategories 整体替换单封邮件的类别列表。
func SetMessageCategories(ctx context.Context, accessToken, messageID string, categories []string) error {
	return patchMessageProperties(ctx, accessToken, messageID, map[string][]string{"categories": categories})

}

// AddMessageCategory 给邮件添加一个类别。
func AddMessageCategory(ctx context.Context, accessToken, messageID, category string) ([]string, error) {
	category = strings.TrimSpace(category)
	if category == "" {
		return nil, fmt.Errorf("类别名称不能为空")
	}
	categories, err := GetMessageCategories(ctx, accessToken, messageID)
	if err != nil {
		return nil, err
	}
	for _, existing := range categories {
		if strings.EqualFold(existing, category) {
			return categories, nil
		}
	}
	categories = append(categories, category)
	if err := SetMessageCategories(ctx, accessToken, messageID, categories); err != nil {
		return nil, err
	}
	return categories, nil
}

// RemoveMessageCategory 从邮件中移除一个类别。
func RemoveMessageCategory(ctx context.Context, accessToken, messageID, category string) ([]string, error) {
	category = strings.TrimSpace(category)
	if category == "" {
		return nil, fmt.Errorf("类别名称不能为空")
	}
	categories, err := GetMessageCategories(ctx, accessToken, messageID)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(categories))
	for _, existing := range categories {
		if !strings.EqualFold(existing, category) {
			result = append(result, existing)
		}
	}
	if err := SetMessageCategories(ctx, accessToken, messageID, result); err != nil {
		return nil, err
	}
	return result, nil
}
