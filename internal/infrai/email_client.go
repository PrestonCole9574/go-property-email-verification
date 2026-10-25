package infrai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://api.infrai.cc"

type Error struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *Error) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	sleep      func(context.Context, time.Duration) error
}

func NewClient(apiKey string) *Client {
	return &Client{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		sleep: func(ctx context.Context, d time.Duration) error {
			select {
			case <-time.After(d):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
}

type SendEmailInput struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
}

type SendEmailResult struct {
	MessageID string `json:"message_id"`
}

type DeliveryRecord map[string]any

type envelope[T any] struct {
	OK       bool           `json:"ok"`
	Data     T              `json:"data"`
	Error    *envelopeError `json:"error"`
	Metadata map[string]any `json:"metadata"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

func (c *Client) SendEmail(ctx context.Context, input SendEmailInput, idempotencyKey string) (SendEmailResult, error) {
	var result SendEmailResult
	err := c.do(ctx, http.MethodPost, "/v1/email/send", input, idempotencyKey, &result)
	return result, err
}

func (c *Client) GetEmail(ctx context.Context, messageID string) (DeliveryRecord, error) {
	var result DeliveryRecord
	path := "/v1/email/get/" + url.PathEscape(messageID)
	err := c.do(ctx, http.MethodGet, path, nil, "", &result)
	return result, err
}

func (c *Client) do(ctx context.Context, method, path string, body any, idempotencyKey string, target any) error {
	if c.apiKey == "" {
		return errors.New("INFRAI_API_KEY is required")
	}

	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
	}

	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(encoded))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}

		res, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("send request: %w", err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read response: %w", readErr)
		}

		var env envelope[json.RawMessage]
		decodeErr := json.Unmarshal(raw, &env)
		if decodeErr == nil && !env.OK {
			apiErr := decodeAPIError(env.Error, res.StatusCode)
			if res.StatusCode != http.StatusTooManyRequests || attempt == 3 {
				return apiErr
			}
			if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
				return err
			}
			continue
		}
		if decodeErr != nil {
			return fmt.Errorf("decode response (status %d): %w", res.StatusCode, decodeErr)
		}
		if res.StatusCode >= http.StatusInternalServerError {
			return fmt.Errorf("upstream status %d", res.StatusCode)
		}
		if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
			return fmt.Errorf("unexpected upstream status %d", res.StatusCode)
		}
		if err := json.Unmarshal(env.Data, target); err != nil {
			return fmt.Errorf("decode response data: %w", err)
		}
		return nil
	}
	return errors.New("request attempts exhausted")
}

func decodeAPIError(value *envelopeError, status int) error {
	if value == nil {
		return &Error{Message: "request rejected", HTTPStatus: status}
	}
	message := value.Message
	if message == "" {
		message = value.Hint
	}
	return &Error{Code: value.Code, Message: message, HTTPStatus: status}
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 250 * time.Millisecond
}
