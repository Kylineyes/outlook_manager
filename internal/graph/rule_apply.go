package graph

import (
	"context"
	"fmt"
	"strings"
)

// RuleApplyMatch 表示一条被规则匹配到的邮件。
type RuleApplyMatch struct {
	ID      string   `json:"id"`
	Subject string   `json:"subject"`
	From    string   `json:"from"`
	Actions []string `json:"actions"`
}

// RuleApplyResult 表示规则回放结果。
type RuleApplyResult struct {
	RuleID   string           `json:"ruleId"`
	RuleName string           `json:"ruleName"`
	FolderID string           `json:"folderId"`
	Scanned  int              `json:"scanned"`
	Matched  int              `json:"matched"`
	Applied  int              `json:"applied"`
	DryRun   bool             `json:"dryRun"`
	Matches  []RuleApplyMatch `json:"matches"`
}

type messageRuleConditions struct {
	FromAddresses         []string
	SubjectContains       []string
	BodyContains          []string
	BodyOrSubjectContains []string
	HasAttachments        bool
	Importance            string
}

type messageRuleActions struct {
	MoveToFolder     string
	MarkAsRead       bool
	Delete           bool
	PermanentDelete  bool
	MarkImportance   string
	AssignCategories []string
}

// ApplyMessageRule 把一条收件箱规则回放到指定文件夹中的已有邮件。
// confirm 为 false 时只做 dry-run，不修改任何邮件。
func ApplyMessageRule(ctx context.Context, accessToken string, rule *MessageRule, folderID string, max int, confirm bool) (*RuleApplyResult, error) {
	if rule == nil {
		return nil, fmt.Errorf("规则不能为空")
	}
	conditions, err := parseMessageRuleConditions(rule.Conditions)
	if err != nil {
		return nil, err
	}
	actions, err := parseMessageRuleActions(rule.Actions)
	if err != nil {
		return nil, err
	}
	folderID = strings.TrimSpace(folderID)
	if folderID == "" {
		folderID = "inbox"
	}
	if max <= 0 {
		max = 500
	}
	if max > 1000 {
		max = 1000
	}
	messages, err := ListMessagesInFolderAll(ctx, accessToken, folderID, max)
	if err != nil {
		return nil, err
	}
	result := &RuleApplyResult{
		RuleID:   rule.ID,
		RuleName: rule.DisplayName,
		FolderID: folderID,
		Scanned:  len(messages),
		DryRun:   !confirm,
		Matches:  make([]RuleApplyMatch, 0),
	}
	for _, message := range messages {
		if !matchesMessageRule(conditions, message) {
			continue
		}
		result.Matched++
		result.Matches = append(result.Matches, RuleApplyMatch{
			ID:      message.ID,
			Subject: message.Subject,
			From:    message.From.EmailAddress.Address,
			Actions: describeMessageRuleActions(actions),
		})
		if !confirm {
			continue
		}
		if err := applyMessageRuleActions(ctx, accessToken, message.ID, actions); err != nil {
			return nil, fmt.Errorf("应用规则到邮件 %s 失败: %w", message.ID, err)
		}
		result.Applied++
	}
	return result, nil
}

func parseMessageRuleConditions(source map[string]any) (*messageRuleConditions, error) {
	conditions := &messageRuleConditions{}
	for key, value := range source {
		switch key {
		case "fromAddresses":
			conditions.FromAddresses = parseRuleAddresses(value)
		case "subjectContains":
			conditions.SubjectContains = parseRuleStringList(value)
		case "bodyContains":
			conditions.BodyContains = parseRuleStringList(value)
		case "bodyOrSubjectContains":
			conditions.BodyOrSubjectContains = parseRuleStringList(value)
		case "hasAttachments":
			if enabled, ok := value.(bool); ok {
				conditions.HasAttachments = enabled
			}
		case "importance":
			conditions.Importance = strings.TrimSpace(toStringValue(value))
		default:
			if !isEmptyRuleValue(value) {
				return nil, fmt.Errorf("规则包含暂不支持的条件: %s", key)
			}
		}
	}
	if len(conditions.FromAddresses) == 0 &&
		len(conditions.SubjectContains) == 0 &&
		len(conditions.BodyContains) == 0 &&
		len(conditions.BodyOrSubjectContains) == 0 &&
		!conditions.HasAttachments &&
		conditions.Importance == "" {
		return nil, fmt.Errorf("规则没有可回放的条件")
	}
	return conditions, nil
}

func parseMessageRuleActions(source map[string]any) (*messageRuleActions, error) {
	actions := &messageRuleActions{}
	for key, value := range source {
		switch key {
		case "moveToFolder":
			actions.MoveToFolder = strings.TrimSpace(toStringValue(value))
		case "markAsRead":
			if enabled, ok := value.(bool); ok {
				actions.MarkAsRead = enabled
			}
		case "delete":
			if enabled, ok := value.(bool); ok {
				actions.Delete = enabled
			}
		case "permanentDelete":
			if enabled, ok := value.(bool); ok {
				actions.PermanentDelete = enabled
			}
		case "markImportance":
			actions.MarkImportance = strings.TrimSpace(toStringValue(value))
		case "assignCategories":
			actions.AssignCategories = parseRuleStringList(value)
		case "stopProcessingRules":
		// 回放单条规则时不需要处理停止后续规则。
		default:
			if !isEmptyRuleValue(value) {
				return nil, fmt.Errorf("规则包含暂不支持的动作: %s", key)
			}
		}
	}
	if actions.MoveToFolder != "" && (actions.Delete || actions.PermanentDelete) {
		return nil, fmt.Errorf("规则同时包含移动和删除动作，拒绝回放以避免歧义")
	}
	if actions.Delete && actions.PermanentDelete {
		return nil, fmt.Errorf("规则同时包含删除和永久删除动作，拒绝回放")
	}
	if actions.MarkImportance != "" {
		switch strings.ToLower(actions.MarkImportance) {
		case "low", "normal", "high":
		default:
			return nil, fmt.Errorf("规则包含不支持的优先级: %s", actions.MarkImportance)
		}
	}
	if actions.MoveToFolder == "" && !actions.MarkAsRead && !actions.Delete && !actions.PermanentDelete && actions.MarkImportance == "" && len(actions.AssignCategories) == 0 {
		return nil, fmt.Errorf("规则没有可回放的动作")
	}
	return actions, nil
}

func matchesMessageRule(conditions *messageRuleConditions, message Message) bool {
	if len(conditions.FromAddresses) > 0 {
		matched := false
		for _, address := range conditions.FromAddresses {
			if strings.EqualFold(strings.TrimSpace(address), strings.TrimSpace(message.From.EmailAddress.Address)) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(conditions.SubjectContains) > 0 && !containsAnyKeyword(message.Subject, conditions.SubjectContains) {
		return false
	}
	if len(conditions.BodyContains) > 0 && !containsAnyKeyword(message.BodyPreview, conditions.BodyContains) {
		return false
	}
	if len(conditions.BodyOrSubjectContains) > 0 && !containsAnyKeyword(message.Subject+"\n"+message.BodyPreview, conditions.BodyOrSubjectContains) {
		return false
	}
	if conditions.HasAttachments && !message.HasAttachments {
		return false
	}
	if conditions.Importance != "" && !strings.EqualFold(conditions.Importance, message.Importance) {
		return false
	}
	return true
}

func applyMessageRuleActions(ctx context.Context, accessToken, messageID string, actions *messageRuleActions) error {
	if actions.MoveToFolder != "" {
		if _, err := MoveMessage(ctx, accessToken, messageID, actions.MoveToFolder); err != nil {
			return err
		}
	}
	for _, category := range actions.AssignCategories {
		if _, err := AddMessageCategory(ctx, accessToken, messageID, category); err != nil {
			return err
		}
	}
	if actions.MarkAsRead {
		if err := SetMessageRead(ctx, accessToken, messageID, true); err != nil {
			return err
		}
	}
	if actions.MarkImportance != "" {
		if err := SetMessageImportance(ctx, accessToken, messageID, actions.MarkImportance); err != nil {
			return err
		}
	}
	if actions.Delete {
		if err := DeleteMessage(ctx, accessToken, messageID); err != nil {
			return err
		}
	}
	if actions.PermanentDelete {
		if err := PermanentDeleteMessage(ctx, accessToken, messageID); err != nil {
			return err
		}
	}
	return nil
}

func describeMessageRuleActions(actions *messageRuleActions) []string {
	result := make([]string, 0)
	if actions.MoveToFolder != "" {
		result = append(result, "moveToFolder:"+actions.MoveToFolder)
	}
	for _, category := range actions.AssignCategories {
		result = append(result, "assignCategory:"+category)
	}
	if actions.MarkAsRead {
		result = append(result, "markAsRead")
	}
	if actions.MarkImportance != "" {
		result = append(result, "markImportance:"+actions.MarkImportance)
	}
	if actions.Delete {
		result = append(result, "delete")
	}
	if actions.PermanentDelete {
		result = append(result, "permanentDelete")
	}
	return result
}

func parseRuleStringList(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(toStringValue(item)); text != "" {
				result = append(result, text)
			}
		}
		return result
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{strings.TrimSpace(typed)}
	default:
		return nil
	}
}

func parseRuleAddresses(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		email, ok := entry["emailAddress"].(map[string]any)
		if !ok {
			continue
		}
		address := strings.TrimSpace(toStringValue(email["address"]))
		if address != "" {
			result = append(result, address)
		}
	}
	return result
}

func toStringValue(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func containsAnyKeyword(text string, keywords []string) bool {
	lowerText := strings.ToLower(text)
	for _, keyword := range keywords {
		keyword = strings.TrimSpace(keyword)
		if keyword != "" && strings.Contains(lowerText, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func isEmptyRuleValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case bool:
		return !typed
	case []any:
		return len(typed) == 0
	case []string:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}
