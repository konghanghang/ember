package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	turnstileLoginAction       = "login"
	turnstileVerifyEndpoint    = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	turnstileRequestTimeout    = 5 * time.Second
	turnstileResponseBodyLimit = 32 * 1024
)

var ErrTurnstileValidationFailed = errors.New("人机校验失败，请重试")

type TurnstileVerifyPayload struct {
	Token            string
	RemoteIP         string
	ExpectedAction   string
	ExpectedHostname string
}

type turnstileVerifyResponse struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes"`
	Hostname   string   `json:"hostname"`
	Action     string   `json:"action"`
}

type turnstileHTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type TurnstileVerifyError struct {
	Reason string
	Err    error
}

func (e *TurnstileVerifyError) Error() string {
	if e == nil {
		return "turnstile verify failed"
	}
	if e.Err == nil {
		return fmt.Sprintf("turnstile verify failed: %s", e.Reason)
	}
	return fmt.Sprintf("turnstile verify failed: %s: %v", e.Reason, e.Err)
}

func (e *TurnstileVerifyError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type CloudflareTurnstileVerifier struct {
	endpoint  string
	secretKey string
	client    turnstileHTTPClient
}

func NewCloudflareTurnstileVerifier() *CloudflareTurnstileVerifier {
	return NewCloudflareTurnstileVerifierWithClient(
		strings.TrimSpace(os.Getenv("TURNSTILE_SECRET_KEY")),
		&http.Client{Timeout: turnstileRequestTimeout},
		turnstileVerifyEndpoint,
	)
}

func NewCloudflareTurnstileVerifierWithClient(secretKey string, client turnstileHTTPClient, endpoint string) *CloudflareTurnstileVerifier {
	if client == nil {
		client = &http.Client{Timeout: turnstileRequestTimeout}
	}
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		endpoint = turnstileVerifyEndpoint
	}

	return &CloudflareTurnstileVerifier{
		endpoint:  endpoint,
		secretKey: strings.TrimSpace(secretKey),
		client:    client,
	}
}

func (v *CloudflareTurnstileVerifier) VerifyLogin(ctx context.Context, payload TurnstileVerifyPayload) error {
	if strings.TrimSpace(payload.Token) == "" {
		return &TurnstileVerifyError{Reason: "missing_token"}
	}
	if v == nil || strings.TrimSpace(v.secretKey) == "" {
		return &TurnstileVerifyError{Reason: "missing_secret_key"}
	}

	form := url.Values{}
	form.Set("secret", v.secretKey)
	form.Set("response", strings.TrimSpace(payload.Token))
	if strings.TrimSpace(payload.RemoteIP) != "" {
		form.Set("remoteip", strings.TrimSpace(payload.RemoteIP))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return &TurnstileVerifyError{Reason: "build_request_failed", Err: err}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.client.Do(req)
	if err != nil {
		return &TurnstileVerifyError{Reason: "request_failed", Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, turnstileResponseBodyLimit))
		return &TurnstileVerifyError{Reason: fmt.Sprintf("unexpected_status_%d", resp.StatusCode)}
	}

	var verifyResp turnstileVerifyResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, turnstileResponseBodyLimit)).Decode(&verifyResp); err != nil {
		return &TurnstileVerifyError{Reason: "decode_response_failed", Err: err}
	}

	if !verifyResp.Success {
		reason := "siteverify_rejected"
		if len(verifyResp.ErrorCodes) > 0 {
			reason = fmt.Sprintf("siteverify_rejected_%s", strings.Join(verifyResp.ErrorCodes, ","))
		}
		return &TurnstileVerifyError{Reason: reason}
	}

	expectedAction := strings.TrimSpace(payload.ExpectedAction)
	if expectedAction != "" && verifyResp.Action != expectedAction {
		return &TurnstileVerifyError{Reason: "action_mismatch"}
	}

	expectedHostname := strings.TrimSpace(payload.ExpectedHostname)
	if expectedHostname != "" && !strings.EqualFold(strings.TrimSpace(verifyResp.Hostname), expectedHostname) {
		return &TurnstileVerifyError{Reason: "hostname_mismatch"}
	}

	return nil
}

func (s *AuthService) verifyTurnstileForLogin(req *LoginRequest) error {
	if !s.isTurnstileLoginEnabled() {
		return nil
	}

	if strings.TrimSpace(s.getConfigString("turnstile_site_key")) == "" {
		log.Printf("登录 Turnstile 校验失败：reason=missing_site_key, clientIP=%s", strings.TrimSpace(req.ClientIP))
		return ErrTurnstileValidationFailed
	}

	verifier := s.turnstileVerifier
	if verifier == nil {
		verifier = NewCloudflareTurnstileVerifier()
	}

	ctx := context.Background()
	if req.RequestContext != nil {
		ctx = req.RequestContext
	}
	payload := TurnstileVerifyPayload{
		Token:            strings.TrimSpace(req.TurnstileToken),
		RemoteIP:         strings.TrimSpace(req.ClientIP),
		ExpectedAction:   turnstileLoginAction,
		ExpectedHostname: strings.TrimSpace(s.getConfigString("turnstile_expected_hostname")),
	}
	if err := verifier.VerifyLogin(ctx, payload); err != nil {
		reason := "verify_failed"
		var verifyErr *TurnstileVerifyError
		if errors.As(err, &verifyErr) && strings.TrimSpace(verifyErr.Reason) != "" {
			reason = verifyErr.Reason
		}
		log.Printf("登录 Turnstile 校验失败：reason=%s, clientIP=%s", reason, strings.TrimSpace(req.ClientIP))
		return ErrTurnstileValidationFailed
	}

	return nil
}

func (s *AuthService) isTurnstileLoginEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(s.getConfigString("turnstile_login_enabled")), "true")
}

func (s *AuthService) getConfigString(key string) string {
	if s.configReader == nil {
		return ""
	}
	return s.configReader.GetString(key)
}
