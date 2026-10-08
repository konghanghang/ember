package telegram

import (
	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	redemptionpkg "github.com/konghang/ember/backend/internal/services/redemption"
	subscriptionpkg "github.com/konghang/ember/backend/internal/services/subscription"
)

type defaultTelegramRedeemer struct{}

// Redeem forwards the granted validity without inferring permanence from a nullable expiry.
func (defaultTelegramRedeemer) Redeem(userID, code string) (*TelegramRedeemResponse, error) {
	resp, err := (&redemptionpkg.RedemptionService{}).RedeemCode(userID, &redemptionpkg.RedeemCodeRequest{Code: code})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, nil
	}
	return &TelegramRedeemResponse{
		ValidityType: resp.ValidityType,
		Message:      resp.Message,
		Days:         resp.Days,
		ExpiresAt:    resp.ExpiresAt,
	}, nil
}

type defaultTelegramSubscriber struct{}

func (defaultTelegramSubscriber) Create(userID string, req TelegramSubscriptionCommand) error {
	return subscriptionpkg.NewSubscriptionService().CreateSubscription(userID, subscriptionpkg.CreateSubscriptionRequest{
		Type:            req.Type,
		Name:            req.Name,
		TmdbID:          req.TmdbID,
		Season:          req.Season,
		PosterPath:      req.PosterPath,
		ConfirmExisting: true,
	})
}

func NewDefaultService() *TelegramService {
	service := NewTelegramService(
		defaultTelegramRedeemer{},
		defaultTelegramSubscriber{},
		embyint.GetSharedService,
	)
	service.accountEntitlements = func(id string) ([]AccountEntitlement, error) {
		rows := []AccountEntitlement{}
		err := db.DB.Table("user_entitlements AS e").Select("e.plan_group, g.name AS plan_group_name, e.validity_type, e.expires_at, e.watch_retention_invalidated_at").Joins("JOIN plan_groups g ON g.key=e.plan_group").Where("e.user_id = ?", id).Order("g.entitlement_rank DESC NULLS LAST, e.plan_group").Scan(&rows).Error
		return rows, err
	}
	service.businessTimezone = configpkg.LoadConfiguredTimezone
	return service
}
