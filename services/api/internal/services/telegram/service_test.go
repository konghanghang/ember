package telegram

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

type stubTelegramRedeemer struct {
	lastUserID string
	lastCode   string
	resp       *TelegramRedeemResponse
	err        error
}

func (s *stubTelegramRedeemer) Redeem(userID, code string) (*TelegramRedeemResponse, error) {
	s.lastUserID = userID
	s.lastCode = code
	return s.resp, s.err
}

type stubTelegramSubscriber struct {
	lastUserID string
	lastReq    TelegramSubscriptionCommand
	err        error
}

func (s *stubTelegramSubscriber) Create(userID string, req TelegramSubscriptionCommand) error {
	s.lastUserID = userID
	s.lastReq = req
	return s.err
}

func TestTelegramServiceRedeemForUserDelegatesToRedeemer(t *testing.T) {
	expected := &TelegramRedeemResponse{Message: "ok"}
	redeemer := &stubTelegramRedeemer{resp: expected}
	service := NewTelegramService(redeemer, &stubTelegramSubscriber{}, nil)

	resp, err := service.redeemForUser("user_1", "CODE123")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp != expected {
		t.Fatalf("expected response pointer to be returned")
	}
	if redeemer.lastUserID != "user_1" || redeemer.lastCode != "CODE123" {
		t.Fatalf("unexpected delegated payload: user=%q code=%q", redeemer.lastUserID, redeemer.lastCode)
	}
}

func TestTelegramServiceRedeemForUserReturnsRedeemerError(t *testing.T) {
	expectedErr := errors.New("redeem failed")
	redeemer := &stubTelegramRedeemer{err: expectedErr}
	service := NewTelegramService(redeemer, &stubTelegramSubscriber{}, nil)

	resp, err := service.redeemForUser("user_1", "CODE123")
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected delegated error, got %v", err)
	}
	if resp != nil {
		t.Fatalf("expected nil response on delegated error, got %+v", resp)
	}
	if redeemer.lastUserID != "user_1" || redeemer.lastCode != "CODE123" {
		t.Fatalf("unexpected delegated payload: user=%q code=%q", redeemer.lastUserID, redeemer.lastCode)
	}
}

func TestTelegramServiceSubscribeForUserDelegatesToSubscriber(t *testing.T) {
	subscriber := &stubTelegramSubscriber{}
	service := NewTelegramService(&stubTelegramRedeemer{}, subscriber, nil)
	poster := "/poster.jpg"

	err := service.subscribeForUser("user_2", TelegramSubscribeRequest{
		Type:   "TV",
		Name:   "Show",
		TmdbID: "123",
		Season: 3,
	}, &poster)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if subscriber.lastUserID != "user_2" {
		t.Fatalf("unexpected delegated user: %q", subscriber.lastUserID)
	}
	if subscriber.lastReq.Type != models.MediaType("TV") ||
		subscriber.lastReq.Name != "Show" ||
		subscriber.lastReq.TmdbID != "123" ||
		subscriber.lastReq.Season != 3 {
		t.Fatalf("unexpected delegated request: %+v", subscriber.lastReq)
	}
	if subscriber.lastReq.PosterPath == nil || *subscriber.lastReq.PosterPath != poster {
		t.Fatalf("unexpected poster path: %+v", subscriber.lastReq.PosterPath)
	}
}

func TestTelegramServiceSubscribeForUserAllowsBlankPosterPath(t *testing.T) {
	subscriber := &stubTelegramSubscriber{}
	service := NewTelegramService(&stubTelegramRedeemer{}, subscriber, nil)

	err := service.subscribeForUser("user_2", TelegramSubscribeRequest{
		Type:   "MOVIE",
		Name:   "Movie",
		TmdbID: "456",
	}, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if subscriber.lastUserID != "user_2" {
		t.Fatalf("unexpected delegated user: %q", subscriber.lastUserID)
	}
	if subscriber.lastReq.Type != models.MediaMovie ||
		subscriber.lastReq.Name != "Movie" ||
		subscriber.lastReq.TmdbID != "456" ||
		subscriber.lastReq.Season != 0 {
		t.Fatalf("unexpected delegated request: %+v", subscriber.lastReq)
	}
	if subscriber.lastReq.PosterPath != nil {
		t.Fatalf("expected nil poster path, got %+v", subscriber.lastReq.PosterPath)
	}
}

func TestTelegramServiceSubscribeForUserReturnsSubscriberError(t *testing.T) {
	expectedErr := errors.New("boom")
	service := NewTelegramService(&stubTelegramRedeemer{}, &stubTelegramSubscriber{err: expectedErr}, nil)

	err := service.subscribeForUser("user_3", TelegramSubscribeRequest{
		Type:   "MOVIE",
		Name:   "Movie",
		TmdbID: "456",
	}, nil)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected delegated error, got %v", err)
	}
}

func TestTelegramServiceGetAccountInfoUsesInjectedLookup(t *testing.T) {
	expiredAt := time.Now().UTC().Add(-time.Hour)
	service := NewTelegramService(&stubTelegramRedeemer{}, &stubTelegramSubscriber{}, nil)
	service.findUserByTelegramID = func(telegramID int64) (*models.User, error) {
		if telegramID != 42 {
			t.Fatalf("unexpected telegram id: %d", telegramID)
		}
		return &models.User{
			ID:           "user_1",
			Username:     "ember",
			Email:        "ember@example.com",
			ExpiresAt:    &expiredAt,
			IsActive:     true,
			EmbyDisabled: true,
		}, nil
	}

	resp, err := service.GetAccountInfo(42)
	if err != nil {
		t.Fatalf("expected account info, got %v", err)
	}
	if resp.Username != "ember" || resp.Email != "ember@example.com" || !resp.IsActive || !resp.EmbyDisabled {
		t.Fatalf("unexpected account info: %+v", resp)
	}
	if resp.ExpiresAt != &expiredAt || !resp.IsExpired {
		t.Fatalf("expected expired account info, got %+v", resp)
	}
}

func TestTelegramServiceGetAccountInfoMapsLookupErrors(t *testing.T) {
	testCases := []struct {
		name    string
		err     error
		wantErr error
	}{
		{name: "not bound", err: gorm.ErrRecordNotFound, wantErr: ErrTelegramNotBound},
		{name: "lookup failure", err: errors.New("database unavailable"), wantErr: errors.New("查询账号信息失败，请稍后重试")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			service := NewTelegramService(&stubTelegramRedeemer{}, &stubTelegramSubscriber{}, nil)
			service.findUserByTelegramID = func(telegramID int64) (*models.User, error) {
				return nil, tc.err
			}

			resp, err := service.GetAccountInfo(42)
			if resp != nil {
				t.Fatalf("expected nil response, got %+v", resp)
			}
			if tc.wantErr == ErrTelegramNotBound {
				if !errors.Is(err, ErrTelegramNotBound) {
					t.Fatalf("expected ErrTelegramNotBound, got %v", err)
				}
				return
			}
			if err == nil || err.Error() != tc.wantErr.Error() {
				t.Fatalf("expected %q, got %v", tc.wantErr.Error(), err)
			}
		})
	}
}

func TestTelegramServiceRedeemByTelegramUsesInjectedLookup(t *testing.T) {
	redeemer := &stubTelegramRedeemer{resp: &TelegramRedeemResponse{Message: "兑换成功", Days: 30}}
	service := NewTelegramService(redeemer, &stubTelegramSubscriber{}, nil)
	service.findUserByTelegramID = func(telegramID int64) (*models.User, error) {
		if telegramID != 42 {
			t.Fatalf("unexpected telegram id: %d", telegramID)
		}
		return &models.User{ID: "user_1"}, nil
	}

	resp, err := service.RedeemByTelegram(42, "RENEW30")
	if err != nil {
		t.Fatalf("expected redeem success, got %v", err)
	}
	if resp == nil || resp.Message != "兑换成功" || resp.Days != 30 {
		t.Fatalf("unexpected redeem response: %+v", resp)
	}
	if redeemer.lastUserID != "user_1" || redeemer.lastCode != "RENEW30" {
		t.Fatalf("unexpected delegated redeem payload: userID=%q code=%q", redeemer.lastUserID, redeemer.lastCode)
	}
}

func TestTelegramServiceRedeemByTelegramMapsLookupErrors(t *testing.T) {
	testCases := []struct {
		name    string
		err     error
		wantErr error
	}{
		{name: "not bound", err: gorm.ErrRecordNotFound, wantErr: ErrTelegramNotBound},
		{name: "lookup failure", err: errors.New("database unavailable"), wantErr: errors.New("兑换失败，请稍后重试")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			redeemer := &stubTelegramRedeemer{}
			service := NewTelegramService(redeemer, &stubTelegramSubscriber{}, nil)
			service.findUserByTelegramID = func(telegramID int64) (*models.User, error) {
				return nil, tc.err
			}

			resp, err := service.RedeemByTelegram(42, "RENEW30")
			if resp != nil {
				t.Fatalf("expected nil response, got %+v", resp)
			}
			if redeemer.lastUserID != "" || redeemer.lastCode != "" {
				t.Fatalf("redeemer must not be called after lookup failure")
			}
			if tc.wantErr == ErrTelegramNotBound {
				if !errors.Is(err, ErrTelegramNotBound) {
					t.Fatalf("expected ErrTelegramNotBound, got %v", err)
				}
				return
			}
			if err == nil || err.Error() != tc.wantErr.Error() {
				t.Fatalf("expected %q, got %v", tc.wantErr.Error(), err)
			}
		})
	}
}

func TestTelegramServiceSubscribeByTelegramUsesInjectedLookup(t *testing.T) {
	subscriber := &stubTelegramSubscriber{}
	service := NewTelegramService(&stubTelegramRedeemer{}, subscriber, nil)
	service.findUserByTelegramID = func(telegramID int64) (*models.User, error) {
		if telegramID != 42 {
			t.Fatalf("unexpected telegram id: %d", telegramID)
		}
		return &models.User{ID: "user_1"}, nil
	}

	err := service.SubscribeByTelegram(TelegramSubscribeRequest{
		TelegramID: 42,
		Type:       "TV",
		Name:       "Show",
		TmdbID:     "123",
		Season:     2,
		PosterPath: "/poster.jpg",
	})
	if err != nil {
		t.Fatalf("expected subscribe success, got %v", err)
	}
	if subscriber.lastUserID != "user_1" {
		t.Fatalf("unexpected delegated user id: %q", subscriber.lastUserID)
	}
	if subscriber.lastReq.Type != models.MediaTV || subscriber.lastReq.Name != "Show" ||
		subscriber.lastReq.TmdbID != "123" || subscriber.lastReq.Season != 2 {
		t.Fatalf("unexpected delegated subscription: %+v", subscriber.lastReq)
	}
	if subscriber.lastReq.PosterPath == nil || *subscriber.lastReq.PosterPath != "/poster.jpg" {
		t.Fatalf("unexpected poster path: %+v", subscriber.lastReq.PosterPath)
	}
}

func TestTelegramServiceSubscribeByTelegramMapsLookupErrors(t *testing.T) {
	testCases := []struct {
		name    string
		err     error
		wantErr error
	}{
		{name: "not bound", err: gorm.ErrRecordNotFound, wantErr: ErrTelegramNotBound},
		{name: "lookup failure", err: errors.New("database unavailable"), wantErr: errors.New("订阅失败，请稍后重试")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			subscriber := &stubTelegramSubscriber{}
			service := NewTelegramService(&stubTelegramRedeemer{}, subscriber, nil)
			service.findUserByTelegramID = func(telegramID int64) (*models.User, error) {
				return nil, tc.err
			}

			err := service.SubscribeByTelegram(TelegramSubscribeRequest{
				TelegramID: 42,
				Type:       "MOVIE",
				Name:       "Movie",
				TmdbID:     "456",
			})
			if subscriber.lastUserID != "" {
				t.Fatalf("subscriber must not be called after lookup failure")
			}
			if tc.wantErr == ErrTelegramNotBound {
				if !errors.Is(err, ErrTelegramNotBound) {
					t.Fatalf("expected ErrTelegramNotBound, got %v", err)
				}
				return
			}
			if err == nil || err.Error() != tc.wantErr.Error() {
				t.Fatalf("expected %q, got %v", tc.wantErr.Error(), err)
			}
		})
	}
}

func TestTelegramServiceGenerateBindCodeUsesInjectedStore(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	service := NewTelegramService(&stubTelegramRedeemer{}, &stubTelegramSubscriber{}, nil)
	service.now = func() time.Time { return now }
	service.generateBindCode = func() string { return "123456" }
	service.findUserByID = func(userID string) (*models.User, error) {
		if userID != "user_1" {
			t.Fatalf("unexpected user id: %s", userID)
		}
		return &models.User{ID: userID}, nil
	}

	var capturedUserID string
	var capturedCode string
	var capturedExpiresAt time.Time
	service.upsertBindCode = func(userID, code string, expiresAt time.Time) error {
		capturedUserID = userID
		capturedCode = code
		capturedExpiresAt = expiresAt
		return nil
	}

	code, expiresAt, err := service.GenerateBindCode("user_1")
	if err != nil {
		t.Fatalf("expected bind code, got %v", err)
	}
	if code != "123456" || capturedCode != "123456" {
		t.Fatalf("unexpected bind code: returned=%q captured=%q", code, capturedCode)
	}
	wantExpiresAt := now.Add(5 * time.Minute)
	if !expiresAt.Equal(wantExpiresAt) || !capturedExpiresAt.Equal(wantExpiresAt) {
		t.Fatalf("unexpected expiresAt: returned=%s captured=%s want=%s", expiresAt, capturedExpiresAt, wantExpiresAt)
	}
	if capturedUserID != "user_1" {
		t.Fatalf("expected bind code to be saved for user_1, got %q", capturedUserID)
	}
}

func TestTelegramServiceGenerateBindCodeRejectsAlreadyBoundUserBeforeSave(t *testing.T) {
	telegramID := int64(42)
	service := NewTelegramService(&stubTelegramRedeemer{}, &stubTelegramSubscriber{}, nil)
	service.findUserByID = func(userID string) (*models.User, error) {
		return &models.User{ID: userID, TelegramID: &telegramID}, nil
	}
	service.upsertBindCode = func(userID, code string, expiresAt time.Time) error {
		t.Fatalf("bind code must not be saved for an already bound user")
		return nil
	}

	code, expiresAt, err := service.GenerateBindCode("user_1")
	if !errors.Is(err, ErrUserAlreadyBoundTelegram) {
		t.Fatalf("expected ErrUserAlreadyBoundTelegram, got %v", err)
	}
	if code != "" || !expiresAt.IsZero() {
		t.Fatalf("expected empty result on failure, got code=%q expiresAt=%s", code, expiresAt)
	}
}

func TestTelegramServiceVerifyBindUsesInjectedBindingStore(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	service := NewTelegramService(&stubTelegramRedeemer{}, &stubTelegramSubscriber{}, nil)
	service.now = func() time.Time { return now }
	service.findActiveBindCodes = func(code string, lookupAt time.Time) ([]models.TelegramBindCode, error) {
		if code != "123456" {
			t.Fatalf("unexpected bind code: %s", code)
		}
		if !lookupAt.Equal(now) {
			t.Fatalf("expected lookup time %s, got %s", now, lookupAt)
		}
		return []models.TelegramBindCode{{UserID: "user_1", Code: code}}, nil
	}
	service.findUserByTelegramID = func(telegramID int64) (*models.User, error) {
		if telegramID != 42 {
			t.Fatalf("unexpected telegram id: %d", telegramID)
		}
		return nil, gorm.ErrRecordNotFound
	}
	service.bindTelegramID = func(userID string, telegramID int64) (*models.User, error) {
		if userID != "user_1" || telegramID != 42 {
			t.Fatalf("unexpected bind payload: userID=%s telegramID=%d", userID, telegramID)
		}
		return &models.User{ID: userID, Username: "ember"}, nil
	}

	result, err := service.VerifyBind(42, "123456")
	if err != nil {
		t.Fatalf("expected bind success, got %v", err)
	}
	if result == nil || result.UserID != "user_1" || result.Username != "ember" {
		t.Fatalf("unexpected bind result: %+v", result)
	}
}

func TestTelegramServiceVerifyBindRejectsAmbiguousBindCodeBeforeMutating(t *testing.T) {
	service := NewTelegramService(&stubTelegramRedeemer{}, &stubTelegramSubscriber{}, nil)
	service.findActiveBindCodes = func(code string, lookupAt time.Time) ([]models.TelegramBindCode, error) {
		return []models.TelegramBindCode{
			{UserID: "user_1", Code: code},
			{UserID: "user_2", Code: code},
		}, nil
	}
	service.findUserByTelegramID = func(telegramID int64) (*models.User, error) {
		t.Fatalf("telegram lookup must not run for ambiguous bind code")
		return nil, nil
	}
	service.bindTelegramID = func(userID string, telegramID int64) (*models.User, error) {
		t.Fatalf("bind mutation must not run for ambiguous bind code")
		return nil, nil
	}

	result, err := service.VerifyBind(42, "123456")
	if !errors.Is(err, ErrTelegramBindCodeInvalid) {
		t.Fatalf("expected ErrTelegramBindCodeInvalid, got %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result, got %+v", result)
	}
}

func TestTelegramServiceVerifyBindRejectsOccupiedTelegramBeforeMutating(t *testing.T) {
	service := NewTelegramService(&stubTelegramRedeemer{}, &stubTelegramSubscriber{}, nil)
	service.findActiveBindCodes = func(code string, lookupAt time.Time) ([]models.TelegramBindCode, error) {
		return []models.TelegramBindCode{{UserID: "user_1", Code: code}}, nil
	}
	service.findUserByTelegramID = func(telegramID int64) (*models.User, error) {
		return &models.User{ID: "other_user", TelegramID: &telegramID}, nil
	}
	service.bindTelegramID = func(userID string, telegramID int64) (*models.User, error) {
		t.Fatalf("bind mutation must not run when telegram id is occupied")
		return nil, nil
	}

	result, err := service.VerifyBind(42, "123456")
	if !errors.Is(err, ErrTelegramAlreadyBound) {
		t.Fatalf("expected ErrTelegramAlreadyBound, got %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result, got %+v", result)
	}
}

func TestTelegramServiceUnbindClearsBoundTelegramID(t *testing.T) {
	telegramID := int64(42)
	service := NewTelegramService(&stubTelegramRedeemer{}, &stubTelegramSubscriber{}, nil)
	service.findUserByID = func(userID string) (*models.User, error) {
		if userID != "user_1" {
			t.Fatalf("unexpected user id: %s", userID)
		}
		return &models.User{ID: userID, TelegramID: &telegramID}, nil
	}

	var clearedUserID string
	service.clearTelegramID = func(userID string) error {
		clearedUserID = userID
		return nil
	}

	if err := service.Unbind("user_1"); err != nil {
		t.Fatalf("expected unbind success, got %v", err)
	}
	if clearedUserID != "user_1" {
		t.Fatalf("expected user_1 telegram id to be cleared, got %q", clearedUserID)
	}
}

func TestTelegramServiceUnbindRejectsUnboundUserBeforeMutation(t *testing.T) {
	service := NewTelegramService(&stubTelegramRedeemer{}, &stubTelegramSubscriber{}, nil)
	service.findUserByID = func(userID string) (*models.User, error) {
		return &models.User{ID: userID}, nil
	}
	service.clearTelegramID = func(userID string) error {
		t.Fatalf("clearTelegramID must not run for an unbound user")
		return nil
	}

	err := service.Unbind("user_1")
	if !errors.Is(err, ErrTelegramNotBound) {
		t.Fatalf("expected ErrTelegramNotBound, got %v", err)
	}
}

func TestTelegramServiceUnbindMapsLookupFailure(t *testing.T) {
	service := NewTelegramService(&stubTelegramRedeemer{}, &stubTelegramSubscriber{}, nil)
	service.findUserByID = func(userID string) (*models.User, error) {
		return nil, errors.New("database unavailable")
	}
	service.clearTelegramID = func(userID string) error {
		t.Fatalf("clearTelegramID must not run after lookup failure")
		return nil
	}

	err := service.Unbind("user_1")
	if !errors.Is(err, ErrTelegramUserNotFound) {
		t.Fatalf("expected ErrTelegramUserNotFound, got %v", err)
	}
}

func TestTelegramServiceResetPasswordMasksLookupFailure(t *testing.T) {
	service := NewTelegramService(&stubTelegramRedeemer{}, &stubTelegramSubscriber{}, nil)
	service.findUserByTelegramID = func(telegramID int64) (*models.User, error) {
		return nil, errors.New("database unavailable")
	}

	err := service.ResetPassword(42, "newpass123")
	if err == nil || err.Error() != "密码重置失败，请稍后重试" {
		t.Fatalf("expected masked lookup failure, got %v", err)
	}
}

func TestTelegramServiceResetPasswordNotBound(t *testing.T) {
	service := NewTelegramService(&stubTelegramRedeemer{}, &stubTelegramSubscriber{}, nil)
	service.findUserByTelegramID = func(telegramID int64) (*models.User, error) {
		return nil, gorm.ErrRecordNotFound
	}

	err := service.ResetPassword(42, "newpass123")
	if !errors.Is(err, ErrTelegramNotBound) {
		t.Fatalf("expected not bound error, got %v", err)
	}
}

func TestTelegramServiceResetPasswordRejectsInvalidLocalPassword(t *testing.T) {
	service := NewTelegramService(&stubTelegramRedeemer{}, &stubTelegramSubscriber{}, nil)
	service.findUserByTelegramID = func(telegramID int64) (*models.User, error) {
		return &models.User{ID: "user_1", Username: "ember"}, nil
	}
	service.saveResetPassword = func(user *models.User) error {
		t.Fatal("saveResetPassword must not be called when password hashing fails")
		return nil
	}

	err := service.ResetPassword(42, strings.Repeat("x", 73))
	if err == nil || err.Error() != "密码重置失败：本地密码更新失败" {
		t.Fatalf("expected local password update failure, got %v", err)
	}
}

func TestTelegramServiceResetPasswordMasksSaveFailure(t *testing.T) {
	service := NewTelegramService(&stubTelegramRedeemer{}, &stubTelegramSubscriber{}, nil)
	service.findUserByTelegramID = func(telegramID int64) (*models.User, error) {
		return &models.User{ID: "user_1", Username: "ember"}, nil
	}
	service.saveResetPassword = func(user *models.User) error {
		return errors.New("database unavailable")
	}

	err := service.ResetPassword(42, "newpass123")
	if err == nil || err.Error() != "密码重置失败：本地密码保存失败" {
		t.Fatalf("expected local password save failure, got %v", err)
	}
}

func TestTelegramServiceResetPasswordUpdatesLocalHash(t *testing.T) {
	service := NewTelegramService(&stubTelegramRedeemer{}, &stubTelegramSubscriber{}, nil)
	service.findUserByTelegramID = func(telegramID int64) (*models.User, error) {
		return &models.User{ID: "user_1", Username: "ember"}, nil
	}
	saved := false
	service.saveResetPassword = func(user *models.User) error {
		saved = true
		if !user.CheckPassword("newpass123") {
			t.Fatalf("expected local password hash to be updated")
		}
		return nil
	}

	if err := service.ResetPassword(42, "newpass123"); err != nil {
		t.Fatalf("expected reset password success, got %v", err)
	}
	if !saved {
		t.Fatalf("expected saveResetPassword to be called")
	}
}
