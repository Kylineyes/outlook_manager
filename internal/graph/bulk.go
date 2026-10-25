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
