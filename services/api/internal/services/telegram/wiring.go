package telegram

import (
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	redemptionpkg "github.com/konghang/ember/backend/internal/services/redemption"
	subscriptionpkg "github.com/konghang/ember/backend/internal/services/subscription"
)

type defaultTelegramRedeemer struct{}

func (defaultTelegramRedeemer) Redeem(userID, code string) (*TelegramRedeemResponse, error) {
	resp, err := (&redemptionpkg.RedemptionService{}).RedeemCode(userID, &redemptionpkg.RedeemCodeRequest{Code: code})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, nil
	}
	return &TelegramRedeemResponse{
		Message:   resp.Message,
		Days:      resp.Days,
		ExpiresAt: resp.ExpiresAt,
	}, nil
}

type defaultTelegramSubscriber struct{}

func (defaultTelegramSubscriber) Create(userID string, req TelegramSubscriptionCommand) error {
	return subscriptionpkg.NewSubscriptionService().CreateSubscription(userID, subscriptionpkg.CreateSubscriptionRequest{
		Type:       req.Type,
		Name:       req.Name,
		TmdbID:     req.TmdbID,
		PosterPath: req.PosterPath,
		Note:       req.Note,
	})
}

func NewDefaultService() *TelegramService {
	return NewTelegramService(
		defaultTelegramRedeemer{},
		defaultTelegramSubscriber{},
		embyint.NewEmbyService,
	)
}
