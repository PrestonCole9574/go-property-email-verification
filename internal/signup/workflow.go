package signup

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"sync"
	"time"

	"github.com/example/property-signup-verification/internal/infrai"
)

var ErrVerificationLink = errors.New("verification link is invalid or already used")

type EmailSender interface {
	SendEmail(context.Context, infrai.SendEmailInput, string) (infrai.SendEmailResult, error)
	GetEmail(context.Context, string) (infrai.DeliveryRecord, error)
}

type SignupInput struct {
	PropertyID string `json:"property_id"`
	TenantID   string `json:"tenant_id"`
	Email      string `json:"email"`
}

type PendingSignup struct {
	PropertyID string `json:"property_id"`
	TenantID   string `json:"tenant_id"`
	Email      string `json:"email"`
	MessageID  string `json:"message_id"`
	State      string `json:"state"`
	tokenHash  [32]byte
}

type MaintenanceRequest struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	State       string `json:"state"`
}

type TenantDocument struct {
	Kind  string `json:"kind"`
	State string `json:"state"`
}

type InspectionReminder struct {
	Kind  string    `json:"kind"`
	DueAt time.Time `json:"due_at"`
}

type TenantWorkspace struct {
	PropertyID          string               `json:"property_id"`
	TenantID            string               `json:"tenant_id"`
	Email               string               `json:"email"`
	State               string               `json:"state"`
	MaintenanceRequests []MaintenanceRequest `json:"maintenance_requests"`
	Documents           []TenantDocument     `json:"documents"`
	InspectionReminders []InspectionReminder `json:"inspection_reminders"`
}

type Workflow struct {
	mail      EmailSender
	publicURL string
	now       func() time.Time
	mu        sync.Mutex
	pending   map[[32]byte]PendingSignup
}

func NewWorkflow(mail EmailSender, publicURL string) *Workflow {
	return &Workflow{
		mail:      mail,
		publicURL: publicURL,
		now:       time.Now,
		pending:   make(map[[32]byte]PendingSignup),
	}
}

func (w *Workflow) Start(ctx context.Context, input SignupInput) (PendingSignup, error) {
	if input.PropertyID == "" || input.TenantID == "" || input.Email == "" {
		return PendingSignup{}, errors.New("property_id, tenant_id, and email are required")
	}
	token, err := randomToken()
	if err != nil {
		return PendingSignup{}, fmt.Errorf("create verification token: %w", err)
	}
	link := w.publicURL + "/verify?token=" + token
	result, err := w.mail.SendEmail(ctx, infrai.SendEmailInput{
		To:      input.Email,
		Subject: "Verify your property account",
		HTML:    "<p>Confirm your email for property " + html.EscapeString(input.PropertyID) + ".</p><p><a href=\"" + html.EscapeString(link) + "\">Verify email</a></p>",
	}, "property-signup:"+input.PropertyID+":"+input.TenantID)
	if err != nil {
		return PendingSignup{}, err
	}
	pending := PendingSignup{
		PropertyID: input.PropertyID,
		TenantID:   input.TenantID,
		Email:      input.Email,
		MessageID:  result.MessageID,
		State:      "verification_sent",
		tokenHash:  sha256.Sum256([]byte(token)),
	}
	w.mu.Lock()
	w.pending[pending.tokenHash] = pending
	w.mu.Unlock()
	return pending, nil
}

func (w *Workflow) Verify(token string) (TenantWorkspace, error) {
	hash := sha256.Sum256([]byte(token))
	w.mu.Lock()
	pending, found := w.pending[hash]
	if found {
		delete(w.pending, hash)
	}
	w.mu.Unlock()
	if !found {
		return TenantWorkspace{}, ErrVerificationLink
	}
	return TenantWorkspace{
		PropertyID:          pending.PropertyID,
		TenantID:            pending.TenantID,
		Email:               pending.Email,
		State:               "active",
		MaintenanceRequests: []MaintenanceRequest{},
		Documents: []TenantDocument{
			{Kind: "lease", State: "requested"},
		},
		InspectionReminders: []InspectionReminder{
			{Kind: "move_in", DueAt: w.now().UTC().Add(72 * time.Hour)},
		},
	}, nil
}

func (w *Workflow) Delivery(ctx context.Context, messageID string) (infrai.DeliveryRecord, error) {
	if messageID == "" {
		return nil, errors.New("message_id is required")
	}
	return w.mail.GetEmail(ctx, messageID)
}

func randomToken() (string, error) {
	value := make([]byte, 24)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
