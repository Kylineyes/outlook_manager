package graph

import (
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
