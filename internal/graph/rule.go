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

// MessageRule 表示 Outlook 收件箱规则。
type MessageRule struct {
	ID          string         `json:"id"`
	DisplayName string         `json:"displayName"`
	Sequence    int            `json:"sequence"`
	IsEnabled   bool           `json:"isEnabled"`
	Conditions  map[string]any `json:"conditions,omitempty"`
	Actions     map[string]any `json:"actions,omitempty"`
}

// MessageRuleInput 是创建收件箱规则的输入。
type MessageRuleInput struct {
	DisplayName         string
	FromAddresses       []string
	SubjectContains     []string
	MoveToFolder        string
	AssignCategories    []string
	MarkAsRead          bool
	Delete              bool
	StopProcessingRules bool
	IsEnabled           bool
}

// buildMessageRulePayload 构造 Graph 收件箱规则请求体。
func buildMessageRulePayload(input MessageRuleInput) (map[string]any, error) {
	displayName := strings.TrimSpace(input.DisplayName)
	if displayName == "" {
		return nil, fmt.Errorf("规则名称不能为空")
	}
	conditions := map[string]any{}
	if len(input.FromAddresses) > 0 {
		addresses := make([]map[string]any, 0, len(input.FromAddresses))
		for _, address := range input.FromAddresses {
			address = strings.TrimSpace(address)
			if address == "" {
				continue
			}
			addresses = append(addresses, map[string]any{
				"emailAddress": map[string]string{"address": address},
			})
		}
		if len(addresses) > 0 {
			conditions["fromAddresses"] = addresses
		}
	}
	if len(input.SubjectContains) > 0 {
		conditions["subjectContains"] = input.SubjectContains
	}
	if len(conditions) == 0 {
		return nil, fmt.Errorf("至少需要一个规则条件")
	}

	actions := map[string]any{}
	if strings.TrimSpace(input.MoveToFolder) != "" {
		actions["moveToFolder"] = strings.TrimSpace(input.MoveToFolder)
	}
	if len(input.AssignCategories) > 0 {
		actions["assignCategories"] = input.AssignCategories
	}
	if input.MarkAsRead {
		actions["markAsRead"] = true
	}
	if input.Delete {
		actions["delete"] = true
	}
	if input.StopProcessingRules {
		actions["stopProcessingRules"] = true
	}
	if len(actions) == 0 {
		return nil, fmt.Errorf("至少需要一个规则动作")
	}

	return map[string]any{
		"displayName": displayName,
		"sequence":    1,
		"isEnabled":   input.IsEnabled,
		"conditions":  conditions,
		"actions":     actions,
	}, nil
}

// ListMessageRules 读取收件箱规则列表。
func ListMessageRules(ctx context.Context, accessToken string) ([]MessageRule, error) {
	endpoint := graphBaseURL + "/me/mailFolders/inbox/messageRules"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建收件箱规则列表请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := doGraphRequest(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 收件箱规则列表失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 收件箱规则列表返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result struct {
		Value []MessageRule `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 Graph 收件箱规则列表失败: %w", err)
	}
	return result.Value, nil
}

// CreateMessageRule 创建收件箱规则。
func CreateMessageRule(ctx context.Context, accessToken string, input MessageRuleInput) (*MessageRule, error) {
	payload, err := buildMessageRulePayload(input)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化创建收件箱规则请求失败: %w", err)
	}
	endpoint := graphBaseURL + "/me/mailFolders/inbox/messageRules"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("创建收件箱规则请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := doGraphRequest(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 创建收件箱规则失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 创建收件箱规则返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var rule MessageRule
	if err := json.NewDecoder(resp.Body).Decode(&rule); err != nil {
		return nil, fmt.Errorf("解析创建收件箱规则响应失败: %w", err)
	}
	return &rule, nil
}

// DeleteMessageRule 删除收件箱规则。
func DeleteMessageRule(ctx context.Context, accessToken, ruleID string) error {
	ruleID = strings.TrimSpace(ruleID)
	if ruleID == "" {
		return fmt.Errorf("规则 ID 不能为空")
	}
	endpoint := graphBaseURL + "/me/mailFolders/inbox/messageRules/" + url.PathEscape(ruleID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("创建删除收件箱规则请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := doGraphRequest(req)
	if err != nil {
		return fmt.Errorf("调用 Graph 删除收件箱规则失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Graph 删除收件箱规则返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// UpdateMessageRule 更新收件箱规则。
func UpdateMessageRule(ctx context.Context, accessToken, ruleID string, input MessageRuleInput) (*MessageRule, error) {
	ruleID = strings.TrimSpace(ruleID)
	if ruleID == "" {
		return nil, fmt.Errorf("规则 ID 不能为空")
	}
	payload, err := buildMessageRulePayload(input)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化更新收件箱规则请求失败: %w", err)
	}
	endpoint := graphBaseURL + "/me/mailFolders/inbox/messageRules/" + url.PathEscape(ruleID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("创建更新收件箱规则请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := doGraphRequest(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 更新收件箱规则失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 更新收件箱规则返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var rule MessageRule
	if err := json.NewDecoder(resp.Body).Decode(&rule); err != nil {
		return nil, fmt.Errorf("解析更新收件箱规则响应失败: %w", err)
	}
	return &rule, nil
}

// GetMessageRule 读取单条收件箱规则。
func GetMessageRule(ctx context.Context, accessToken, ruleID string) (*MessageRule, error) {
	ruleID = strings.TrimSpace(ruleID)
	if ruleID == "" {
		return nil, fmt.Errorf("规则 ID 不能为空")
	}
	endpoint := graphBaseURL + "/me/mailFolders/inbox/messageRules/" + url.PathEscape(ruleID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建收件箱规则详情请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := doGraphRequest(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 收件箱规则详情失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 收件箱规则详情返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var rule MessageRule
	if err := json.NewDecoder(resp.Body).Decode(&rule); err != nil {
		return nil, fmt.Errorf("解析 Graph 收件箱规则详情失败: %w", err)
	}
	return &rule, nil
}
