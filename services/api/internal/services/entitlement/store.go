package entitlement

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Load reads authorizations without interpreting expiry; callers explicitly reconcile at business boundaries.
func Load(tx *gorm.DB, userID string) ([]Holding, error) {
	var rows []models.UserEntitlement
	if err := tx.Where("user_id = ?", userID).Order("plan_group").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Holding, 0, len(rows))
	for _, row := range rows {
		result = append(result, Holding{row.PlanGroup, row.ValidityType, row.ExpiresAt})
	}
	return result, nil
}

// Ranks reads only explicitly configured ranks; it never infers priority from presentation order.
func Ranks(tx *gorm.DB) (map[string]int, error) {
	var groups []models.PlanGroup
	if err := tx.Select("key", "entitlement_rank").Find(&groups).Error; err != nil {
		return nil, err
	}
	ranks := make(map[string]int, len(groups))
	for _, group := range groups {
		if group.EntitlementRank != nil {
			ranks[group.Key] = *group.EntitlementRank
		}
	}
	return ranks, nil
}

// GrantLocked grants an immutable bundle under a user row lock owned by the caller's transaction.
// The event key makes replay harmless; external side effects must run only after commit.
func GrantLocked(tx *gorm.DB, user *models.User, benefits []Benefit, sourceKey, actor string, at time.Time, loc *time.Location) error {
	if sourceKey == "" || actor == "" {
		return errors.New("权益来源与操作人不能为空")
	}
	var event models.EntitlementEvent
	err := tx.Where("source_key = ?", sourceKey).First(&event).Error
	if err == nil {
		if event.UserID != user.ID {
			return errors.New("权益来源已被其他用户使用")
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	before, err := Load(tx, user.ID)
	if err != nil {
		return err
	}
	ranks, err := Ranks(tx)
	if err != nil {
		return err
	}
	// Before administrators configure ranks, registration/renewal of one known
	// group remains possible. No ordering is invented between different groups.
	if len(ranks) == 0 || missingRank(before, benefits, ranks) {
		single := ""
		for _, holding := range before {
			if single != "" && single != holding.PlanGroup {
				return ErrGroupsNotReady
			}
			single = holding.PlanGroup
		}
		for _, benefit := range benefits {
			if single != "" && single != benefit.PlanGroup {
				return ErrGroupsNotReady
			}
			single = benefit.PlanGroup
		}
		var count int64
		if err := tx.Model(&models.PlanGroup{}).Where("key = ?", single).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return ErrGroupsNotReady
		}
		ranks[single] = 0
	}
	after, err := Grant(before, benefits, ranks, at, loc)
	if err != nil {
		return err
	}
	return saveLocked(tx, user, before, after, ranks, sourceKey, actor, "grant", at)
}

// missingRank detects undefined ordering before evaluating a grant.
func missingRank(owned []Holding, benefits []Benefit, ranks map[string]int) bool {
	for _, h := range owned {
		if _, ok := ranks[h.PlanGroup]; !ok {
			return true
		}
	}
	for _, b := range benefits {
		if _, ok := ranks[b.PlanGroup]; !ok {
			return true
		}
	}
	return false
}

// saveLocked persists grants, their audit and the selected group atomically in the caller's transaction.
func saveLocked(tx *gorm.DB, user *models.User, before, after []Holding, ranks map[string]int, key, actor, reason string, at time.Time) error {
	if err := tx.Where("user_id = ?", user.ID).Delete(&models.UserEntitlement{}).Error; err != nil {
		return err
	}
	for _, holding := range after {
		row := models.UserEntitlement{UserID: user.ID, PlanGroup: holding.PlanGroup, ValidityType: holding.ValidityType, ExpiresAt: holding.ExpiresAt}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
	}
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		return err
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		return err
	}
	if err := tx.Create(&models.EntitlementEvent{SourceKey: key, UserID: user.ID, Actor: actor, Reason: reason, BeforeState: string(beforeJSON), AfterState: string(afterJSON), CreatedAt: at}).Error; err != nil {
		return err
	}
	_, err = projectLocked(tx, user, after, ranks, at)
	if err == nil {
		log.Printf("[Entitlement] 权益已写入事务 userId=%s reason=%s groupCount=%d", user.ID, reason, len(after))
	}
	return err
}

// projectLocked updates access without touching manual bans or remote synchronization status.
func projectLocked(tx *gorm.DB, user *models.User, owned []Holding, ranks map[string]int, at time.Time) (bool, error) {
	selected, err := Resolve(owned, ranks, at)
	if err != nil {
		return false, err
	}
	granted := selected != nil
	group := user.PlanGroup
	expires := user.ExpiresAt
	if selected != nil {
		group = &selected.PlanGroup
		expires = selected.ExpiresAt
	}
	changed := user.ResourceAccessGranted != granted || !sameString(user.PlanGroup, group) || !sameTime(user.ExpiresAt, expires)
	if !changed {
		return false, nil
	}
	if err := tx.Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]interface{}{
		"plan_group": group, "expires_at": expires, "resource_access_granted": granted,
	}).Error; err != nil {
		return false, err
	}
	user.PlanGroup, user.ExpiresAt, user.ResourceAccessGranted = group, expires, granted
	return true, nil
}

// Reconcile locks and re-reads one user for the existing scheduled expiry job.
func Reconcile(database *gorm.DB, userID string, at time.Time) (*models.User, bool, error) {
	var user models.User
	var changed bool
	err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", userID).First(&user).Error; err != nil {
			return err
		}
		if user.IsAdmin() {
			return nil
		}
		owned, err := Load(tx, user.ID)
		if err != nil {
			return err
		}
		ranks, err := Ranks(tx)
		if err != nil {
			return err
		}
		if len(owned) == 1 {
			if _, ok := ranks[owned[0].PlanGroup]; !ok {
				ranks[owned[0].PlanGroup] = 0
			}
		}
		changed, err = projectLocked(tx, &user, owned, ranks, at)
		return err
	})
	return &user, changed, err
}

// ReconcileLocked allows a group-rank change to commit all local projections in the same transaction.
// The caller owns the user locks and supplies the complete, validated rank mapping.
func ReconcileLocked(tx *gorm.DB, user *models.User, ranks map[string]int, at time.Time) (bool, error) {
	if user.IsAdmin() {
		return false, nil
	}
	owned, err := Load(tx, user.ID)
	if err != nil {
		return false, err
	}
	return projectLocked(tx, user, owned, ranks, at)
}

// sameString compares optional projection values without pointer identity.
func sameString(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

// sameTime compares optional deadlines by instant, irrespective of storage timezone.
func sameTime(a, b *time.Time) bool {
	return a == nil && b == nil || a != nil && b != nil && a.Equal(*b)
}

// AdjustLocked persists one explicit administrator adjustment under the caller's user lock.
func AdjustLocked(tx *gorm.DB, user *models.User, target Holding, revoke bool, key, actor string, at time.Time) error {
	if key == "" || actor == "" {
		return errors.New("权益来源与操作人不能为空")
	}
	var event models.EntitlementEvent
	err := tx.Where("source_key = ?", key).First(&event).Error
	if err == nil {
		if event.UserID != user.ID {
			return errors.New("权益来源已被其他用户使用")
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var group models.PlanGroup
	if err := tx.Where("key = ?", target.PlanGroup).First(&group).Error; err != nil {
		return err
	}
	before, err := Load(tx, user.ID)
	if err != nil {
		return err
	}
	after, err := Adjust(before, target, revoke)
	if err != nil {
		return err
	}
	ranks, err := Ranks(tx)
	if err != nil {
		return err
	}
	if len(after) == 1 {
		if _, ok := ranks[after[0].PlanGroup]; !ok {
			ranks[after[0].PlanGroup] = 0
		}
	}
	if _, err := Resolve(after, ranks, at); err != nil {
		return err
	}
	reason := "admin_set"
	if revoke {
		reason = "admin_revoke"
	}
	return saveLocked(tx, user, before, after, ranks, key, actor, reason, at)
}

// ValidateCatalog checks all configured group ranks and adjacent library supersets before selling.
func ValidateCatalog(tx *gorm.DB) error {
	var groups []models.PlanGroup
	if err := tx.Order("entitlement_rank ASC NULLS LAST").Find(&groups).Error; err != nil {
		return err
	}
	var libraries []models.PlanGroupMediaLibrary
	if err := tx.Find(&libraries).Error; err != nil {
		return err
	}
	sets := map[string]map[string]bool{}
	for _, group := range groups {
		sets[group.Key] = map[string]bool{}
	}
	for _, library := range libraries {
		if sets[library.PlanGroupKey] != nil {
			sets[library.PlanGroupKey][library.LibraryID] = true
		}
	}
	if len(groups) == 0 {
		return ErrGroupsNotReady
	}
	for i, group := range groups {
		if group.EntitlementRank == nil || *group.EntitlementRank < 0 {
			return ErrGroupsNotReady
		}
		if i == 0 {
			continue
		}
		previous := groups[i-1]
		if *previous.EntitlementRank == *group.EntitlementRank {
			return ErrGroupsNotReady
		}
		for library := range sets[previous.Key] {
			if !sets[group.Key][library] {
				return fmt.Errorf("%w: %s 未包含 %s 的全部媒体库", ErrGroupsNotReady, group.Key, previous.Key)
			}
		}
	}
	return nil
}
