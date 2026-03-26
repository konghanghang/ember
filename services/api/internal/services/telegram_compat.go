package services

import (
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	redemptionpkg "github.com/konghang/ember/backend/internal/services/redemption"
	subscriptionpkg "github.com/konghang/ember/backend/internal/services/subscription"
	telegrampkg "github.com/konghang/ember/backend/internal/services/telegram"
)

type TelegramService = telegrampkg.TelegramService
type BindResult = telegrampkg.BindResult
type AccountInfoResponse = telegrampkg.AccountInfoResponse
type TelegramBindRequest = telegrampkg.TelegramBindRequest
type TelegramRedeemRequest = telegrampkg.TelegramRedeemRequest
type TelegramRedeemResponse = telegrampkg.TelegramRedeemResponse
type TelegramResetPasswordRequest = telegrampkg.TelegramResetPasswordRequest
type TelegramIDRequest = telegrampkg.TelegramIDRequest
type TelegramSubscribeRequest = telegrampkg.TelegramSubscribeRequest
type TelegramSubscriptionCommand = telegrampkg.TelegramSubscriptionCommand

var (
	ErrTelegramAlreadyBound     = telegrampkg.ErrTelegramAlreadyBound
	ErrTelegramBindCodeInvalid  = telegrampkg.ErrTelegramBindCodeInvalid
	ErrTelegramNotBound         = telegrampkg.ErrTelegramNotBound
	ErrUserAlreadyBoundTelegram = telegrampkg.ErrUserAlreadyBoundTelegram
)

type telegramRedeemerAdapter struct{}

func (telegramRedeemerAdapter) Redeem(userID, code string) (*telegrampkg.TelegramRedeemResponse, error) {
	resp, err := (&redemptionpkg.RedemptionService{}).RedeemCode(userID, &redemptionpkg.RedeemCodeRequest{Code: code})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, nil
	}
	return &telegrampkg.TelegramRedeemResponse{
		Message:   resp.Message,
		Days:      resp.Days,
		ExpiresAt: resp.ExpiresAt,
	}, nil
}

type telegramSubscriberAdapter struct{}

func (telegramSubscriberAdapter) Create(userID string, req telegrampkg.TelegramSubscriptionCommand) error {
	return subscriptionpkg.NewSubscriptionService().CreateSubscription(userID, subscriptionpkg.CreateSubscriptionRequest{
		Type:       req.Type,
		Name:       req.Name,
		TmdbID:     req.TmdbID,
		PosterPath: req.PosterPath,
		Note:       req.Note,
	})
}

func NewTelegramService() *TelegramService {
	return telegrampkg.NewTelegramService(
		telegramRedeemerAdapter{},
		telegramSubscriberAdapter{},
		embyint.NewEmbyService,
	)
}
