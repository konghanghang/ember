package redemption

import (
	"errors"
	"testing"
	"time"
)

func TestRedeemCodeUsesInjectedStoreWithTrimmedCode(t *testing.T) {
	expiresAt := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	var capturedUserID string
	var capturedCode string
	service := &RedemptionService{
		redeemCodeStore: func(userID string, req *RedeemCodeRequest) (*RedeemCodeResponse, error) {
			capturedUserID = userID
			capturedCode = req.Code
			return &RedeemCodeResponse{
				Message:   "兑换成功，有效期已延长 30 天",
				Days:      30,
				ExpiresAt: &expiresAt,
			}, nil
		},
	}

	resp, err := service.RedeemCode("user_1", &RedeemCodeRequest{Code: "  invite-code  "})

	if err != nil {
		t.Fatalf("expected redeem success, got %v", err)
	}
	if capturedUserID != "user_1" || capturedCode != "invite-code" {
		t.Fatalf("unexpected store args: userID=%s code=%s", capturedUserID, capturedCode)
	}
	if resp == nil || resp.Days != 30 || resp.ExpiresAt == nil || !resp.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("unexpected redeem response: %+v", resp)
	}
}

func TestRedeemCodePropagatesInjectedStoreError(t *testing.T) {
	service := &RedemptionService{
		redeemCodeStore: func(userID string, req *RedeemCodeRequest) (*RedeemCodeResponse, error) {
			return nil, ErrRedemptionCodeInvalid
		},
	}

	resp, err := service.RedeemCode("user_1", &RedeemCodeRequest{Code: "bad-code"})

	if !errors.Is(err, ErrRedemptionCodeInvalid) {
		t.Fatalf("expected ErrRedemptionCodeInvalid, got resp=%+v err=%v", resp, err)
	}
	if resp != nil {
		t.Fatalf("expected nil response on redeem failure, got %+v", resp)
	}
}

func TestCalculateRedeemedExpiryStartsFromNowWithoutActiveExpiry(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	expiredAt := now.Add(-2 * time.Hour)

	tests := []struct {
		name          string
		currentExpiry *time.Time
	}{
		{name: "nil expiry", currentExpiry: nil},
		{name: "expired expiry", currentExpiry: &expiredAt},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateRedeemedExpiry(now, tt.currentExpiry, 30)
			want := now.AddDate(0, 0, 30)

			if !got.Equal(want) {
				t.Fatalf("expected expiry from now %s, got %s", want, got)
			}
		})
	}
}

func TestCalculateRedeemedExpiryExtendsActiveExpiry(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	currentExpiry := now.AddDate(0, 0, 12)

	got := calculateRedeemedExpiry(now, &currentExpiry, 45)
	want := currentExpiry.AddDate(0, 0, 45)

	if !got.Equal(want) {
		t.Fatalf("expected active expiry extension %s, got %s", want, got)
	}
}
