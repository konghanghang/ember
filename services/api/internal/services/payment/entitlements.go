package payment

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	entitlementpkg "github.com/konghang/ember/backend/internal/services/entitlement"
	policypkg "github.com/konghang/ember/backend/internal/services/policy"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrManualResolutionInvalid = errors.New("人工处理参数无效或订单不处于待处理状态")

// SetEntitlementRanksRequest requires a complete mapping so omitted groups cannot silently change precedence.
type SetEntitlementRanksRequest struct {
	Ranks map[string]int `json:"ranks" binding:"required"`
}

// SetEntitlementRanks atomically updates ranks and all local projections; remote synchronization follows commit.
func (s *PaymentService) SetEntitlementRanks(ctx context.Context, req SetEntitlementRanksRequest) error {
	changedUsers := []string{}
	err := db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock users before groups, matching grant/FK ordering and preventing a
		// partially updated set of effective groups if a later user write fails.
		var users []models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("role = ?", "user").Order("id").Find(&users).Error; err != nil {
			return err
		}
		var groups []models.PlanGroup
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("key").Find(&groups).Error; err != nil {
			return err
		}
		if len(groups) != len(req.Ranks) {
			return entitlementpkg.ErrGroupsNotReady
		}
		seen := map[int]bool{}
		for _, group := range groups {
			rank, ok := req.Ranks[group.Key]
			if !ok || rank < 0 || seen[rank] {
				return entitlementpkg.ErrGroupsNotReady
			}
			seen[rank] = true
		}
		if err := tx.Model(&models.PlanGroup{}).Where("1 = 1").Update("entitlement_rank", nil).Error; err != nil {
			return err
		}
		for key, rank := range req.Ranks {
			if err := tx.Model(&models.PlanGroup{}).Where("key = ?", key).Update("entitlement_rank", rank).Error; err != nil {
				return err
			}
		}
		if err := entitlementpkg.ValidateCatalog(tx); err != nil {
			return err
		}
		ranks, err := entitlementpkg.Ranks(tx)
		if err != nil {
			return err
		}
		at := time.Now()
		for i := range users {
			changed, err := entitlementpkg.ReconcileLocked(tx, &users[i], ranks, at)
			if err != nil {
				return err
			}
			if changed {
				changedUsers = append(changedUsers, users[i].ID)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, id := range changedUsers {
		if err := policypkg.NewService(s.embyService).ApplyEffectiveUserPolicyOrRecordFailure(id, "entitlement_rank_changed"); err != nil {
			log.Printf("[Entitlement] 等级变更同步失败 userId=%s err=%v", id, err)
		}
	}
	return nil
}

// ResolvePaymentRequest records an externally completed refund or explicitly grants compensation.
type ResolvePaymentRequest struct {
	Resolution string               `json:"resolution" binding:"required,oneof=external_refund compensation"`
	Note       string               `json:"note" binding:"required,max=500"`
	Benefits   []models.PlanBenefit `json:"benefits"`
}

// ResolvePayment serializes with payment fulfillment and atomically records compensation and resolution.
func (s *PaymentService) ResolvePayment(ctx context.Context, id, actor string, req ResolvePaymentRequest) error {
	if strings.TrimSpace(req.Note) == "" || utf8.RuneCountInString(req.Note) > 500 || (req.Resolution != "external_refund" && req.Resolution != "compensation") {
		return ErrManualResolutionInvalid
	}
	location := s.businessTimezone()
	var ref models.Payment
	if err := db.DB.WithContext(ctx).Select("id", "user_id").Where("id = ?", id).First(&ref).Error; err != nil {
		return err
	}
	var user models.User
	changed := false
	err := db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", ref.UserID).First(&user).Error; err != nil {
			return err
		}
		var payment models.Payment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", id, user.ID).First(&payment).Error; err != nil {
			return err
		}
		if payment.Status == models.PaymentResolved {
			return nil
		}
		if payment.Status != models.PaymentManualReview {
			return ErrManualResolutionInvalid
		}
		if req.Resolution == "compensation" {
			if err := entitlementpkg.GrantLocked(tx, &user, req.Benefits, "payment-resolution:"+id, actor, time.Now(), location); err != nil {
				return err
			}
			changed = true
		}
		return tx.Model(&models.Payment{}).Where("id = ?", id).Updates(map[string]interface{}{"status": models.PaymentResolved, "resolution": req.Resolution, "resolution_note": strings.TrimSpace(req.Note), "resolved_by": actor, "resolved_at": time.Now()}).Error
	})
	if err != nil {
		return err
	}
	if changed {
		return policypkg.NewService(s.embyService).ApplyEffectiveUserPolicyOrRecordFailure(user.ID, "payment_manual_compensation")
	}
	return nil
}
