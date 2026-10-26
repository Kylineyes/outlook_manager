package graph

import (
	"context"
	"fmt"
	"strings"
)

// BulkResult 表示批量操作结果。
type BulkResult struct {
	Total     int      `json:"total"`
	Succeeded int      `json:"succeeded"`
	Failed    int      `json:"failed"`
	DryRun    bool     `json:"dryRun"`
	Failures  []string `json:"failures,omitempty"`
}

func normalizeBulkIDs(ids []string) []string {
	result := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
	}
	return result
}

// BulkMoveMessages 批量移动邮件。
func BulkMoveMessages(ctx context.Context, accessToken string, ids []string, folderID string, confirm bool) (*BulkResult, error) {
	folderID = strings.TrimSpace(folderID)
	if folderID == "" {
		return nil, fmt.Errorf("目标文件夹不能为空")
	}
	ids = normalizeBulkIDs(ids)
	result := &BulkResult{Total: len(ids), DryRun: !confirm}
	if !confirm {
		return result, nil
	}
	for _, id := range ids {
		if _, err := MoveMessage(ctx, accessToken, id, folderID); err != nil {
			result.Failed++
			result.Failures = append(result.Failures, fmt.Sprintf("%s: %v", id, err))
			continue
		}
		result.Succeeded++
	}
	return result, nil
}

// BulkDeleteMessages 批量删除邮件（移动到已删除邮件）。
func BulkDeleteMessages(ctx context.Context, accessToken string, ids []string, confirm bool) (*BulkResult, error) {
	ids = normalizeBulkIDs(ids)
	result := &BulkResult{Total: len(ids), DryRun: !confirm}
	if !confirm {
		return result, nil
	}
	for _, id := range ids {
		if err := DeleteMessage(ctx, accessToken, id); err != nil {
			result.Failed++
			result.Failures = append(result.Failures, fmt.Sprintf("%s: %v", id, err))
			continue
		}
		result.Succeeded++
	}
	return result, nil
}

// BulkDeleteContacts 批量删除联系人。
func BulkDeleteContacts(ctx context.Context, accessToken string, ids []string, confirm bool) (*BulkResult, error) {
	ids = normalizeBulkIDs(ids)
	result := &BulkResult{Total: len(ids), DryRun: !confirm}
	if !confirm {
		return result, nil
	}
	for _, id := range ids {
		if err := DeleteContact(ctx, accessToken, id); err != nil {
			result.Failed++
			result.Failures = append(result.Failures, fmt.Sprintf("%s: %v", id, err))
			continue
		}
		result.Succeeded++
	}
	return result, nil
}

// BulkDeleteCalendarEventAttachments 批量删除日历事件附件。
func BulkDeleteCalendarEventAttachments(ctx context.Context, accessToken, eventID string, ids []string, confirm bool) (*BulkResult, error) {
	ids = normalizeBulkIDs(ids)
	result := &BulkResult{Total: len(ids), DryRun: !confirm}
	if !confirm {
		return result, nil
	}
	for _, id := range ids {
		if err := DeleteCalendarEventAttachment(ctx, accessToken, eventID, id); err != nil {
			result.Failed++
			result.Failures = append(result.Failures, fmt.Sprintf("%s: %v", id, err))
			continue
		}
		result.Succeeded++
	}
	return result, nil
}

// BulkDeleteDraftAttachments 批量删除草稿附件。
func BulkDeleteDraftAttachments(ctx context.Context, accessToken, draftID string, ids []string, confirm bool) (*BulkResult, error) {
	ids = normalizeBulkIDs(ids)
	result := &BulkResult{Total: len(ids), DryRun: !confirm}
	if !confirm {
		return result, nil
	}
	for _, id := range ids {
		if err := DeleteDraftAttachment(ctx, accessToken, draftID, id); err != nil {
			result.Failed++
			result.Failures = append(result.Failures, fmt.Sprintf("%s: %v", id, err))
			continue
		}
		result.Succeeded++
	}
	return result, nil
}

// BulkSetMessagesRead 批量设置邮件已读/未读状态。
func BulkSetMessagesRead(ctx context.Context, accessToken string, ids []string, isRead, confirm bool) (*BulkResult, error) {
	ids = normalizeBulkIDs(ids)
	result := &BulkResult{Total: len(ids), DryRun: !confirm}
	if !confirm {
		return result, nil
	}
	for _, id := range ids {
		if err := SetMessageRead(ctx, accessToken, id, isRead); err != nil {
			result.Failed++
			result.Failures = append(result.Failures, fmt.Sprintf("%s: %v", id, err))
			continue
		}
		result.Succeeded++
	}
	return result, nil
}

// BulkSetMessagesFlag 批量设置/取消邮件旗标。
func BulkSetMessagesFlag(ctx context.Context, accessToken string, ids []string, flagged, confirm bool) (*BulkResult, error) {
	ids = normalizeBulkIDs(ids)
	result := &BulkResult{Total: len(ids), DryRun: !confirm}
	if !confirm {
		return result, nil
	}
	for _, id := range ids {
		if err := SetMessageFlag(ctx, accessToken, id, flagged); err != nil {
			result.Failed++
			result.Failures = append(result.Failures, fmt.Sprintf("%s: %v", id, err))
			continue
		}
		result.Succeeded++
	}
	return result, nil
}

// BulkSetMessagesImportance 批量设置邮件重要性。
func BulkSetMessagesImportance(ctx context.Context, accessToken string, ids []string, importance string, confirm bool) (*BulkResult, error) {
	importance = strings.ToLower(strings.TrimSpace(importance))
	switch importance {
	case "low", "normal", "high":
	default:
		return nil, fmt.Errorf("不支持的邮件重要性: %s", importance)
	}
	ids = normalizeBulkIDs(ids)
	result := &BulkResult{Total: len(ids), DryRun: !confirm}
	if !confirm {
		return result, nil
	}
	for _, id := range ids {
		if err := SetMessageImportance(ctx, accessToken, id, importance); err != nil {
			result.Failed++
			result.Failures = append(result.Failures, fmt.Sprintf("%s: %v", id, err))
			continue
		}
		result.Succeeded++
	}
	return result, nil
}

// BulkAddMessagesCategory 批量给邮件添加类别。
func BulkAddMessagesCategory(ctx context.Context, accessToken string, ids []string, category string, confirm bool) (*BulkResult, error) {
	category = strings.TrimSpace(category)
	if category == "" {
		return nil, fmt.Errorf("类别名称不能为空")
	}
	ids = normalizeBulkIDs(ids)
	result := &BulkResult{Total: len(ids), DryRun: !confirm}
	if !confirm {
		return result, nil
	}
	for _, id := range ids {
		if _, err := AddMessageCategory(ctx, accessToken, id, category); err != nil {
			result.Failed++
			result.Failures = append(result.Failures, fmt.Sprintf("%s: %v", id, err))
			continue
		}
		result.Succeeded++
	}
	return result, nil
}

// BulkRemoveMessagesCategory 批量移除邮件类别。
func BulkRemoveMessagesCategory(ctx context.Context, accessToken string, ids []string, category string, confirm bool) (*BulkResult, error) {
	category = strings.TrimSpace(category)
	if category == "" {
		return nil, fmt.Errorf("类别名称不能为空")
	}
	ids = normalizeBulkIDs(ids)
	result := &BulkResult{Total: len(ids), DryRun: !confirm}
	if !confirm {
		return result, nil
	}
	for _, id := range ids {
		if _, err := RemoveMessageCategory(ctx, accessToken, id, category); err != nil {
			result.Failed++
			result.Failures = append(result.Failures, fmt.Sprintf("%s: %v", id, err))
			continue
		}
		result.Succeeded++
	}
	return result, nil
}
