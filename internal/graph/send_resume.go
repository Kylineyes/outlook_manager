package graph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/Kylineyes/outlook-manager/internal/uploadstate"
)

type sendResumeAttachment struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type sendResumeFingerprint struct {
	To          []string               `json:"to"`
	Cc          []string               `json:"cc"`
	Bcc         []string               `json:"bcc"`
	Subject     string                 `json:"subject"`
	Body        string                 `json:"body"`
	ContentType string                 `json:"contentType"`
	Attachments []sendResumeAttachment `json:"attachments"`
}

func sendResumeKey(input SendMailInput) string {
	fingerprint := sendResumeFingerprint{
		To: input.To, Cc: input.Cc, Bcc: input.Bcc,
		Subject: input.Subject, Body: input.Body, ContentType: input.ContentType,
	}
	for _, attachment := range input.Attachments {
		fingerprint.Attachments = append(fingerprint.Attachments, sendResumeAttachment{Name: attachment.Name, Path: attachment.Path, Size: attachment.Size})
	}
	data, _ := json.Marshal(fingerprint)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sendMailWithLargeAttachments(ctx context.Context, accessToken string, input SendMailInput) error {
	key := sendResumeKey(input)
	state, err := uploadstate.LoadSend(key)
	if err != nil {
		return err
	}
	draftID := ""
	if state != nil && state.DraftID != "" {
		if _, getErr := GetMessage(ctx, accessToken, state.DraftID); getErr == nil {
			draftID = state.DraftID
		} else {
			_ = uploadstate.DeleteSend(key)
			state = nil
		}
	}
	if draftID == "" {
		draftID, err = createDraftSkeleton(ctx, accessToken, input)
		if err != nil {
			return err
		}
		state = &uploadstate.SendState{Key: key, DraftID: draftID}
		if err := uploadstate.SaveSend(*state); err != nil {
			_ = DeleteMessage(ctx, accessToken, draftID)
			return err
		}
	}
	if state == nil {
		return fmt.Errorf("发送状态初始化失败")
	}
	if !state.UploadsDone {
		if err := uploadLargeAttachmentsToDraft(ctx, accessToken, draftID, input.Attachments); err != nil {
			return err
		}
		state.UploadsDone = true
		if err := uploadstate.SaveSend(*state); err != nil {
			return err
		}
	}
	if err := SendDraft(ctx, accessToken, draftID); err != nil {
		return err
	}
	return uploadstate.DeleteSend(key)
}
