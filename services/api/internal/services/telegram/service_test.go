package telegram

import (
	"errors"
	"strings"
	"testing"

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
