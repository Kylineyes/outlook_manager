package graph

import (
	"bytes"
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

// CreateMasterCategory 创建主类别。
func CreateMasterCategory(ctx context.Context, accessToken, displayName, color string) (*Category, error) {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return nil, fmt.Errorf("类别名称不能为空")
	}
	payload := map[string]string{"displayName": displayName}
	if strings.TrimSpace(color) != "" {
		payload["color"] = strings.TrimSpace(color)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化创建主类别请求失败: %w", err)
	}
	endpoint := graphBaseURL + "/me/outlook/masterCategories"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("创建主类别请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 创建主类别失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 创建主类别返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var category Category
	if err := json.NewDecoder(resp.Body).Decode(&category); err != nil {
		return nil, fmt.Errorf("解析创建主类别响应失败: %w", err)
	}
	return &category, nil
}

// DeleteMasterCategory 删除主类别。
func DeleteMasterCategory(ctx context.Context, accessToken, categoryID string) error {
	categoryID = strings.TrimSpace(categoryID)
	if categoryID == "" {
		return fmt.Errorf("类别 ID 不能为空")
	}
	endpoint := graphBaseURL + "/me/outlook/masterCategories/" + url.PathEscape(categoryID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("创建删除主类别请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("调用 Graph 删除主类别失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Graph 删除主类别返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
