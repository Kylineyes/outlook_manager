package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ContactEmailAddress 表示联系人邮箱地址。
type ContactEmailAddress struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// Contact 表示一个 Outlook 联系人。
type Contact struct {
	ID             string                `json:"id"`
	DisplayName    string                `json:"displayName"`
	EmailAddresses []ContactEmailAddress `json:"emailAddresses"`
	CompanyName    string                `json:"companyName"`
	JobTitle       string                `json:"jobTitle"`
	MobilePhone    string                `json:"mobilePhone"`
	BusinessPhones []string              `json:"businessPhones"`
}

// ListContacts 读取当前账号的联系人列表。
func ListContacts(ctx context.Context, accessToken string, top int) ([]Contact, error) {
	if top <= 0 {
		top = 50
	}
	if top > 200 {
		top = 200
	}

	query := url.Values{}
	query.Set("$top", strconv.Itoa(top))
	query.Set("$select", "id,displayName,emailAddresses,companyName,jobTitle,mobilePhone,businessPhones")
	query.Set("$orderby", "displayName")
	endpoint := graphBaseURL + "/me/contacts?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建联系人列表请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 联系人列表失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 联系人列表返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		Value []Contact `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 Graph 联系人列表失败: %w", err)
	}
	return result.Value, nil
}

// ContactInput 是创建或更新联系人的输入。
type ContactInput struct {
	DisplayName    string
	GivenName      string
	Surname        string
	EmailAddresses []ContactEmailAddress
	CompanyName    string
	JobTitle       string
	MobilePhone    string
	BusinessPhones []string
}

// buildContactPayload 构造 Graph 联系人请求体。
func buildContactPayload(input ContactInput) map[string]any {
	payload := map[string]any{
		"displayName": input.DisplayName,
	}
	if strings.TrimSpace(input.GivenName) != "" {
		payload["givenName"] = input.GivenName
	}
	if strings.TrimSpace(input.Surname) != "" {
		payload["surname"] = input.Surname
	}
	if len(input.EmailAddresses) > 0 {
		payload["emailAddresses"] = input.EmailAddresses
	}
	if strings.TrimSpace(input.CompanyName) != "" {
		payload["companyName"] = input.CompanyName
	}
	if strings.TrimSpace(input.JobTitle) != "" {
		payload["jobTitle"] = input.JobTitle
	}
	if strings.TrimSpace(input.MobilePhone) != "" {
		payload["mobilePhone"] = input.MobilePhone
	}
	if len(input.BusinessPhones) > 0 {
		payload["businessPhones"] = input.BusinessPhones
	}
	return payload
}

// CreateContact 创建联系人。
func CreateContact(ctx context.Context, accessToken string, input ContactInput) (*Contact, error) {
	data, err := json.Marshal(buildContactPayload(input))
	if err != nil {
		return nil, fmt.Errorf("序列化创建联系人请求失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, graphBaseURL+"/me/contacts", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("创建联系人请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 创建联系人失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 创建联系人返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var contact Contact
	if err := json.NewDecoder(resp.Body).Decode(&contact); err != nil {
		return nil, fmt.Errorf("解析创建联系人响应失败: %w", err)
	}
	return &contact, nil
}

// UpdateContact 更新联系人。
func UpdateContact(ctx context.Context, accessToken, contactID string, input ContactInput) (*Contact, error) {
	contactID = strings.TrimSpace(contactID)
	if contactID == "" {
		return nil, fmt.Errorf("联系人 ID 不能为空")
	}
	data, err := json.Marshal(buildContactPayload(input))
	if err != nil {
		return nil, fmt.Errorf("序列化更新联系人请求失败: %w", err)
	}
	endpoint := graphBaseURL + "/me/contacts/" + url.PathEscape(contactID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("创建更新联系人请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 更新联系人失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 更新联系人返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var contact Contact
	if err := json.NewDecoder(resp.Body).Decode(&contact); err != nil {
		return nil, fmt.Errorf("解析更新联系人响应失败: %w", err)
	}
	return &contact, nil
}

// DeleteContact 删除联系人。
func DeleteContact(ctx context.Context, accessToken, contactID string) error {
	contactID = strings.TrimSpace(contactID)
	if contactID == "" {
		return fmt.Errorf("联系人 ID 不能为空")
	}
	endpoint := graphBaseURL + "/me/contacts/" + url.PathEscape(contactID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("创建删除联系人请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("调用 Graph 删除联系人失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Graph 删除联系人返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// SearchContacts 按显示名称前缀搜索联系人。
func SearchContacts(ctx context.Context, accessToken, query string, top int) ([]Contact, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("搜索关键字不能为空")
	}
	if top <= 0 {
		top = 20
	}
	if top > 200 {
		top = 200
	}

	escaped := strings.ReplaceAll(query, "'", "''")
	filter := fmt.Sprintf("startswith(displayName,'%s')", escaped)
	values := url.Values{}
	values.Set("$filter", filter)
	values.Set("$top", strconv.Itoa(top))
	values.Set("$select", "id,displayName,emailAddresses,companyName,jobTitle,mobilePhone,businessPhones")
	values.Set("$orderby", "displayName")
	endpoint := graphBaseURL + "/me/contacts?" + values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建联系人搜索请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 联系人搜索失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 联系人搜索返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result struct {
		Value []Contact `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 Graph 联系人搜索失败: %w", err)
	}
	return result.Value, nil
}
