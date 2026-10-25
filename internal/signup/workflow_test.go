package signup

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/example/property-signup-verification/internal/infrai"
)

type fakeMail struct {
	sent           infrai.SendEmailInput
	idempotencyKey string
	requestedID    string
}

func (f *fakeMail) SendEmail(_ context.Context, input infrai.SendEmailInput, key string) (infrai.SendEmailResult, error) {
	f.sent = input
	f.idempotencyKey = key
	return infrai.SendEmailResult{MessageID: "msg_42"}, nil
}

func (f *fakeMail) GetEmail(_ context.Context, messageID string) (infrai.DeliveryRecord, error) {
	f.requestedID = messageID
	return infrai.DeliveryRecord{"message_id": messageID, "status": "delivered"}, nil
}

func TestVerificationControlsWorkspaceProvisioning(t *testing.T) {
	tests := []struct {
		name        string
		token       func(string) string
		wantState   string
		wantErr     bool
		wantDocs    int
		wantReminds int
	}{
		{name: "valid link activates tenant resources", token: func(value string) string { return value }, wantState: "active", wantDocs: 1, wantReminds: 1},
		{name: "changed link provisions nothing", token: func(value string) string { return value + "changed" }, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mail := &fakeMail{}
			workflow := NewWorkflow(mail, "http://localhost:8080")
			workflow.now = func() time.Time { return time.Date(2026, 9, 8, 9, 0, 0, 0, time.UTC) }
			pending, err := workflow.Start(context.Background(), SignupInput{PropertyID: "building-7", TenantID: "tenant-9", Email: "tenant@example.com"})
			if err != nil {
				t.Fatal(err)
			}
			parts := strings.Split(mail.sent.HTML, "token=")
			if len(parts) != 2 {
				t.Fatalf("verification token missing from email: %s", mail.sent.HTML)
			}
			token := strings.Split(parts[1], "\"")[0]

			workspace, err := workflow.Verify(tt.token(token))
			if (err != nil) != tt.wantErr {
				t.Fatalf("Verify() error = %v, wantErr %v", err, tt.wantErr)
			}
			if workspace.State != tt.wantState || len(workspace.Documents) != tt.wantDocs || len(workspace.InspectionReminders) != tt.wantReminds {
				t.Fatalf("workspace = %#v", workspace)
			}
			if !tt.wantErr && len(workspace.MaintenanceRequests) != 0 {
				t.Fatalf("new workspace has maintenance requests: %#v", workspace.MaintenanceRequests)
			}
			if pending.MessageID != "msg_42" || mail.idempotencyKey != "property-signup:building-7:tenant-9" {
				t.Fatalf("send handoff = %#v, key = %q", pending, mail.idempotencyKey)
			}
		})
	}
}

func TestDeliveryUsesSendMessageID(t *testing.T) {
	mail := &fakeMail{}
	workflow := NewWorkflow(mail, "http://localhost:8080")
	pending, err := workflow.Start(context.Background(), SignupInput{PropertyID: "p1", TenantID: "t1", Email: "tenant@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	record, err := workflow.Delivery(context.Background(), pending.MessageID)
	if err != nil {
		t.Fatal(err)
	}
	if mail.requestedID != "msg_42" || record["status"] != "delivered" {
		t.Fatalf("delivery handoff id = %q, record = %#v", mail.requestedID, record)
	}
}
