package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Kylineyes/outlook-manager/internal/audit"
)

// MailboxSettings 表示 Microsoft Graph 的用户邮箱设置。
type MailboxSettings struct {
	AutomaticRepliesSetting AutomaticRepliesSetting `json:"automaticRepliesSetting"`
	TimeZone                string                  `json:"timeZone"`
	Language                MailboxLanguage         `json:"language"`
	WorkDays                []string                `json:"workDays,omitempty"`
	WorkingHours            *WorkingHours           `json:"workingHours,omitempty"`
}

// AutomaticRepliesSetting 表示自动回复设置。
type AutomaticRepliesSetting struct {
	Status                 string            `json:"status"`
	ExternalAudience       string            `json:"externalAudience"`
	IsEnabled              bool              `json:"isEnabled,omitempty"`
	ScheduledStartDateTime *DateTimeTimeZone `json:"scheduledStartDateTime,omitempty"`
	ScheduledEndDateTime   *DateTimeTimeZone `json:"scheduledEndDateTime,omitempty"`
	InternalReplyMessage   string            `json:"internalReplyMessage,omitempty"`
	ExternalReplyMessage   string            `json:"externalReplyMessage,omitempty"`
}

// MailboxLanguage 表示邮箱语言设置。
type MailboxLanguage struct {
	DisplayName string `json:"displayName,omitempty"`
	Locale      string `json:"locale,omitempty"`
}

// WorkingHours 表示工作时间设置。
type WorkingHours struct {
	StartTime string `json:"startTime,omitempty"`
	EndTime   string `json:"endTime,omitempty"`
	TimeZone  struct {
		Name string `json:"name,omitempty"`
	} `json:"timeZone,omitempty"`
}

// GetMailboxSettings 读取当前账号的邮箱设置。
func GetMailboxSettings(ctx context.Context, accessToken string) (*MailboxSettings, error) {
	endpoint := graphBaseURL + "/me/mailboxSettings"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建邮箱设置请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := doGraphRequest(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 读取邮箱设置失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 读取邮箱设置返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var settings MailboxSettings
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		return nil, fmt.Errorf("解析邮箱设置失败: %w", err)
	}
	return &settings, nil
}

func patchMailboxSettings(ctx context.Context, accessToken string, payload any) (*MailboxSettings, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化邮箱设置请求失败: %w", err)
	}
	endpoint := graphBaseURL + "/me/mailboxSettings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("创建邮箱设置更新请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := doGraphRequest(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 更新邮箱设置失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 更新邮箱设置返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var settings MailboxSettings
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		return nil, fmt.Errorf("解析更新后的邮箱设置失败: %w", err)
	}
	return &settings, nil
}

// SetMailboxTimeZone 更新邮箱时区。
func SetMailboxTimeZone(ctx context.Context, accessToken, timeZone string) (*MailboxSettings, error) {
	timeZone = strings.TrimSpace(timeZone)
	if timeZone == "" {
		return nil, fmt.Errorf("时区不能为空")
	}
	settings, err := patchMailboxSettings(ctx, accessToken, map[string]string{"timeZone": timeZone})
	if err != nil {
		return nil, err
	}
	audit.Log("mailbox.timezone.set", map[string]any{"timeZone": timeZone})
	return settings, nil
}

// SetAutomaticReplies 更新自动回复设置。
func SetAutomaticReplies(ctx context.Context, accessToken string, setting AutomaticRepliesSetting) (*MailboxSettings, error) {
	status := strings.ToLower(strings.TrimSpace(setting.Status))
	if strings.TrimSpace(setting.ExternalAudience) == "" {
		setting.ExternalAudience = "all"
	}
	switch status {
	case "", "alwaysenabled":
		setting.Status = "alwaysEnabled"
		setting.IsEnabled = true
	case "disabled":
		setting.Status = "disabled"
		setting.IsEnabled = false
	case "scheduled":
		setting.Status = "scheduled"
		setting.IsEnabled = true
	default:
		return nil, fmt.Errorf("不支持的自动回复状态: %s", setting.Status)
	}
	payload := map[string]any{"automaticRepliesSetting": setting}
	settings, err := patchMailboxSettings(ctx, accessToken, payload)
	if err != nil {
		return nil, err
	}
	audit.Log("mailbox.autoReply.set", map[string]any{"status": setting.Status, "internal": setting.InternalReplyMessage != "", "external": setting.ExternalReplyMessage != ""})
	return settings, nil
}
