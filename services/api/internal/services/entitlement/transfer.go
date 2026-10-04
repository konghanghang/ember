package entitlement

import (
	"encoding/json"
	"errors"
	"log"
	"sort"
	"strings"

	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

var (
	ErrTransferSourceMissing = errors.New("用户未持有原分组权益")
	ErrTransferTargetExists  = errors.New("用户已持有目标分组权益，不能覆盖或合并")
	ErrTransferTargetMissing = errors.New("目标分组不存在")
)

// Transfer replaces one holding's group without changing deadlines, validity or other holdings.
// No clock/rank resolution runs: expired rights stay expired and permanent rights stay permanent.
func Transfer(owned []Holding, source, target string) ([]Holding, error) {
	if strings.TrimSpace(source) == "" || strings.TrimSpace(target) == "" || source == target || source != strings.TrimSpace(source) || target != strings.TrimSpace(target) {
		return nil, ErrInvalidHolding
	}
	index := -1
	for i, h := range owned {
		if h.PlanGroup == target {
			return nil, ErrTransferTargetExists
		}
		if h.PlanGroup == source {
			index = i
		}
	}
	if index < 0 {
		return nil, ErrTransferSourceMissing
	}
	after := append([]Holding(nil), owned...)
	after[index].PlanGroup = target
	sort.Slice(after, func(i, j int) bool { return after[i].PlanGroup < after[j].PlanGroup })
	return after, nil
}

// TransferLocked atomically renames the grant and current-group projection under the caller's user lock.
// History and access/expiry flags remain untouched; the caller synchronizes Policy after commit.
func TransferLocked(tx *gorm.DB, user *models.User, source, target, key, actor string) error {
	if key == "" || actor == "" {
		return ErrInvalidHolding
	}
	var err error
	var targetGroup models.PlanGroup
	if err = tx.Where("key = ?", target).First(&targetGroup).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTransferTargetMissing
		}
		return err
	}
	before, err := Load(tx, user.ID)
	if err != nil {
		return err
	}
	after, err := Transfer(before, source, target)
	if err != nil {
		return err
	}
	result := tx.Model(&models.UserEntitlement{}).Where("user_id = ? AND plan_group = ?", user.ID, source).Update("plan_group", target)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrTransferSourceMissing
	}
	if user.PlanGroup != nil && *user.PlanGroup == source {
		if err = tx.Model(&models.User{}).Where("id = ?", user.ID).Update("plan_group", target).Error; err != nil {
			return err
		}
		user.PlanGroup = &target
	}
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		return err
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		return err
	}
	if err = tx.Create(&models.EntitlementEvent{SourceKey: key, UserID: user.ID, Actor: actor, Reason: "admin_transfer", BeforeState: string(beforeJSON), AfterState: string(afterJSON)}).Error; err != nil {
		return err
	}
	log.Printf("[Entitlement] 更换分组已写入事务 userId=%s sourceGroup=%s targetGroup=%s", user.ID, source, target)
	return nil
}
