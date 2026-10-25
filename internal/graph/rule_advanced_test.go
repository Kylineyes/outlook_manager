package graph

import (
	"testing"
)

func TestBuildMessageRulePayloadAdvanced(t *testing.T) {
	payload, err := buildMessageRulePayload(MessageRuleInput{
		DisplayName:       "高级规则",
		BodyContains:      []string{"关键字"},
		HasAttachments:    true,
		Importance:        "high",
		SenderContains:    []string{"bank"},
		RecipientContains: []string{"me"},
		CopyToFolder:      "copy-folder",
		ForwardTo:         []string{"forward@example.com"},
		RedirectTo:        []string{"redirect@example.com"},
		MarkImportance:    "low",
		PermanentDelete:   true,
		IsEnabled:         true,
	})
	if err != nil {
		t.Fatalf("buildMessageRulePayload returned error: %v", err)
	}
	conditions := payload["conditions"].(map[string]any)
	actions := payload["actions"].(map[string]any)
	if conditions["bodyContains"] == nil || conditions["hasAttachments"] != true || conditions["importance"] != "high" {
		t.Fatalf("unexpected conditions: %#v", conditions)
	}
	if conditions["senderContains"] == nil || conditions["recipientContains"] == nil {
		t.Fatalf("missing sender/recipient conditions: %#v", conditions)
	}
	if actions["copyToFolder"] != "copy-folder" || actions["markImportance"] != "low" || actions["permanentDelete"] != true {
		t.Fatalf("unexpected actions: %#v", actions)
	}
	if actions["forwardTo"] == nil || actions["redirectTo"] == nil {
		t.Fatalf("missing forward/redirect actions: %#v", actions)
	}
}

func TestMatchesMessageRuleSenderRecipient(t *testing.T) {
	message := Message{
		From:         MessageFrom{EmailAddress: EmailAddress{Name: "Bank Service", Address: "bank@example.com"}},
		ToRecipients: []Recipient{{EmailAddress: EmailAddress{Name: "Me", Address: "me@example.com"}}},
	}
	conditions := &messageRuleConditions{
		SenderContains:    []string{"bank"},
		RecipientContains: []string{"me@example.com"},
	}
	if !matchesMessageRule(conditions, message, "") {
		t.Fatal("expected sender/recipient match")
	}
	message.ToRecipients[0].EmailAddress.Address = "other@example.com"
	if matchesMessageRule(conditions, message, "") {
		t.Fatal("expected recipient mismatch")
	}
}
