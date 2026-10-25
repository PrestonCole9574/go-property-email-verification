package infrai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientDecodesEnvelopeBeforeStatusAndRetriesRateLimit(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if r.Method != http.MethodPost || r.URL.Path != "/v1/email/send" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("Idempotency-Key") != "signup-1" {
			t.Fatalf("headers = %#v", r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"ok":false,"data":null,"error":{"code":"RATE_LIMITED","message":"retry later"},"metadata":{}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"data":{"message_id":"msg_7"},"error":null,"metadata":{"vendor":"mail"}}`))
	}))
	defer server.Close()

	client := NewClient("test-key")
	client.baseURL = server.URL
	client.sleep = func(context.Context, time.Duration) error { return nil }
	result, err := client.SendEmail(context.Background(), SendEmailInput{To: "tenant@example.com", Subject: "Verify", HTML: "<p>Verify</p>"}, "signup-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.MessageID != "msg_7" || attempts != 2 {
		t.Fatalf("result = %#v, attempts = %d", result, attempts)
	}
}

func TestClientReturnsBusinessEnvelopeOnClientStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"data":null,"error":{"code":"INVALID_INPUT","message":"email is required"},"metadata":{}}`))
	}))
	defer server.Close()

	client := NewClient("test-key")
	client.baseURL = server.URL
	_, err := client.SendEmail(context.Background(), SendEmailInput{Subject: "Verify", HTML: "<p>Verify</p>"}, "signup-2")
	apiErr, ok := err.(*Error)
	if !ok || apiErr.Code != "INVALID_INPUT" || apiErr.HTTPStatus != http.StatusBadRequest {
		t.Fatalf("error = %#v", err)
	}
}
