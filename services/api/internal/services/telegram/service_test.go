package telegram

import (
	"errors"
	"testing"

	"github.com/konghang/ember/backend/internal/models"
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

func TestTelegramServiceSubscribeForUserDelegatesToSubscriber(t *testing.T) {
	subscriber := &stubTelegramSubscriber{}
	service := NewTelegramService(&stubTelegramRedeemer{}, subscriber, nil)
	poster := "/poster.jpg"
	note := "hello"

	err := service.subscribeForUser("user_2", TelegramSubscribeRequest{
		Type:   "TV",
		Name:   "Show",
		TmdbID: "123",
	}, &poster, &note)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if subscriber.lastUserID != "user_2" {
		t.Fatalf("unexpected delegated user: %q", subscriber.lastUserID)
	}
	if subscriber.lastReq.Type != models.MediaType("TV") ||
		subscriber.lastReq.Name != "Show" ||
		subscriber.lastReq.TmdbID != "123" {
		t.Fatalf("unexpected delegated request: %+v", subscriber.lastReq)
	}
	if subscriber.lastReq.PosterPath == nil || *subscriber.lastReq.PosterPath != poster {
		t.Fatalf("unexpected poster path: %+v", subscriber.lastReq.PosterPath)
	}
	if subscriber.lastReq.Note == nil || *subscriber.lastReq.Note != note {
		t.Fatalf("unexpected note: %+v", subscriber.lastReq.Note)
	}
}

func TestTelegramServiceSubscribeForUserReturnsSubscriberError(t *testing.T) {
	expectedErr := errors.New("boom")
	service := NewTelegramService(&stubTelegramRedeemer{}, &stubTelegramSubscriber{err: expectedErr}, nil)

	err := service.subscribeForUser("user_3", TelegramSubscribeRequest{
		Type:   "MOVIE",
		Name:   "Movie",
		TmdbID: "456",
	}, nil, nil)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected delegated error, got %v", err)
	}
}
