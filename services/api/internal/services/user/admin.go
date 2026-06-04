package user

import (
	"errors"
	"log"
	"net/mail"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	paymentpkg "github.com/konghang/ember/backend/internal/services/payment"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const embyAdminPolicyProtectionText = "There must be at least one user in the system with administrative access"

// AdminUpdateUserRequest 管理员更新用户请求
type AdminUpdateUserRequest struct {
	Email          *string `json:"email"`
	IsActive       *bool   `json:"isActive"`
	PlanGroup      *string `json:"planGroup"`
	ExpiresAt      *string `json:"expiresAt"`
	ClearExpiresAt bool    `json:"clearExpiresAt"`
}

// GetUsersRequest 获取用户列表请求
type GetUsersRequest struct {
	Page         int    `form:"page" binding:"omitempty,min=1"`
	PageSize     int    `form:"pageSize" binding:"omitempty,min=1"`
	Search       string `form:"search"`
	IsActive     *bool  `form:"isActive"`
	ExpiresAfter string `form:"expiresAfter"`
	EmbyStatus   string `form:"embyStatus"`
	PlanGroup    string `form:"planGroup"`
}

// GetUsersResponse 获取用户列表响应
type GetUsersResponse struct {
	Data       []UserView `json:"data"`
	Total      int64      `json:"total"`
	Page       int        `json:"page"`
	PageSize   int        `json:"pageSize"`
	TotalPages int        `json:"totalPages"`
}

// ExtendExpiryRequest 延长到期时间请求
type ExtendExpiryRequest struct {
	Days int `json:"days" binding:"required,min=1"`
}

func buildUsersWithPlanGroupSelect(query *gorm.DB) *gorm.DB {
	adminPolicyProtectionPattern := "%" + embyAdminPolicyProtectionText + "%"
	return query.
		Select(`users.*,
			explicit_pg.name AS "planGroupName",
			COALESCE(users."plan_group", default_pg.key) AS "effectivePlanGroup",
			CASE
				WHEN users."plan_group" IS NULL THEN default_pg.name
				WHEN explicit_pg.key IS NULL THEN ''
				ELSE explicit_pg.name
			END AS "effectivePlanGroupName",
			CASE
				WHEN users."plan_group" IS NOT NULL AND explicit_pg.key IS NULL THEN true
				ELSE false
			END AS "isPlanGroupMissing",
			EXISTS (
				SELECT 1 FROM user_media_library_preferences prefs
				WHERE prefs.user_id = users.id
			) AS "mediaLibraryPreferenceCustomized",
			(
				SELECT COUNT(*) FROM plan_group_media_libraries libs
				WHERE libs.plan_group_key = COALESCE(users."plan_group", default_pg.key)
				  AND LOWER(COALESCE(libs.library_type, '')) <> 'boxsets'
			) AS "mediaLibraryTemplateCount",
			CASE
				WHEN EXISTS (
					SELECT 1 FROM user_media_library_preferences prefs
					WHERE prefs.user_id = users.id
				) THEN (
					SELECT COUNT(*) FROM user_media_library_preferences prefs
					JOIN plan_group_media_libraries libs
					  ON libs.library_id = prefs.library_id
					 AND libs.plan_group_key = COALESCE(users."plan_group", default_pg.key)
					 AND LOWER(COALESCE(libs.library_type, '')) <> 'boxsets'
					WHERE prefs.user_id = users.id
					  AND prefs.enabled = true
				)
				ELSE (
					SELECT COUNT(*) FROM plan_group_media_libraries libs
					WHERE libs.plan_group_key = COALESCE(users."plan_group", default_pg.key)
					  AND LOWER(COALESCE(libs.library_type, '')) <> 'boxsets'
				)
			END AS "mediaLibraryEnabledCount",
			CASE
				WHEN users.role <> 'user' THEN 'synced'
				WHEN EXISTS (
					SELECT 1 FROM emby_policy_sync_tasks tasks
					WHERE tasks.user_id = users.id AND tasks.status = 'processing'
				) THEN 'processing'
				WHEN EXISTS (
					SELECT 1 FROM emby_policy_sync_tasks tasks
					WHERE tasks.user_id = users.id AND tasks.status = 'pending'
				) THEN 'pending'
				WHEN EXISTS (
					SELECT 1 FROM emby_policy_sync_tasks tasks
					WHERE tasks.user_id = users.id AND tasks.status = 'failed'
					  AND tasks.batch_id IS NULL
					  AND COALESCE(tasks.last_error, '') NOT LIKE ?
				) THEN 'failed'
				ELSE 'synced'
			END AS "policySyncStatus",
			CASE
				WHEN users.role <> 'user' THEN ''
				WHEN EXISTS (
					SELECT 1 FROM emby_policy_sync_tasks tasks
					WHERE tasks.user_id = users.id AND tasks.status = 'failed'
					  AND tasks.batch_id IS NOT NULL
					  AND COALESCE(tasks.last_error, '') NOT LIKE ?
				) THEN 'failed'
				ELSE ''
			END AS "policySyncBatchStatus",
			COALESCE((
				SELECT tasks.batch_id
				FROM emby_policy_sync_tasks tasks
				WHERE tasks.user_id = users.id AND tasks.status = 'failed'
				  AND tasks.batch_id IS NOT NULL
				  AND COALESCE(tasks.last_error, '') NOT LIKE ?
				ORDER BY tasks.updated_at DESC, tasks.created_at DESC
				LIMIT 1
			), '') AS "policySyncBatchId"`, adminPolicyProtectionPattern, adminPolicyProtectionPattern, adminPolicyProtectionPattern).
		Joins(`LEFT JOIN plan_groups explicit_pg ON explicit_pg.key = users."plan_group"`).
		Joins(`LEFT JOIN plan_groups default_pg ON default_pg."is_default" = ?`, true)
}

func markUsersUsingDefaultPlanGroup(users []UserView) {
	for i := range users {
		users[i].markUsingDefaultPlanGroup()
	}
}

func (s *UserService) GetUsers(req *GetUsersRequest) (*GetUsersResponse, error) {
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}
	if _, err := paymentpkg.GetDefaultPlanGroup(nil); err != nil {
		return nil, err
	}

	query := db.DB.Model(&models.User{})
	if req.Search != "" {
		query = query.Where("username LIKE ? OR email LIKE ?", "%"+req.Search+"%", "%"+req.Search+"%")
	}
	if req.IsActive != nil {
		query = query.Where("\"is_active\" = ?", *req.IsActive)
	}
	if req.ExpiresAfter != "" {
		expiresAfter, err := time.Parse("2006-01-02", req.ExpiresAfter)
		if err != nil {
			return nil, ErrInvalidExpiresAfter
		}
		query = query.Where("\"expires_at\" IS NOT NULL AND \"expires_at\" > ?", expiresAfter.UTC())
	}

	switch strings.TrimSpace(req.EmbyStatus) {
	case "":
	case "available":
		query = query.Where("COALESCE(\"emby_id\", '') <> '' AND \"emby_disabled\" = ?", false)
	case "disabled":
		query = query.Where("COALESCE(\"emby_id\", '') <> '' AND \"emby_disabled\" = ?", true)
	case "unlinked":
		query = query.Where("COALESCE(\"emby_id\", '') = ''")
	default:
		return nil, ErrInvalidEmbyStatus
	}

	if strings.TrimSpace(req.PlanGroup) != "" {
		defaultGroup, err := paymentpkg.GetDefaultPlanGroup(nil)
		if err != nil {
			return nil, err
		}
		planGroup, err := normalizePlanGroupStrict(req.PlanGroup)
		if err != nil {
			return nil, err
		}
		if defaultGroup.Key == planGroup {
			query = query.Where(`("plan_group" = ? OR "plan_group" IS NULL)`, planGroup)
		} else {
			query = query.Where(`"plan_group" = ?`, planGroup)
		}
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	var users []UserView
	offset := (req.Page - 1) * req.PageSize
	if err := buildUsersWithPlanGroupSelect(query).
		Offset(offset).
		Limit(req.PageSize).
		Order(`users."created_at" DESC`).
		Find(&users).Error; err != nil {
		return nil, err
	}
	markUsersUsingDefaultPlanGroup(users)

	totalPages := int(total) / req.PageSize
	if int(total)%req.PageSize > 0 {
		totalPages++
	}

	return &GetUsersResponse{
		Data:       users,
		Total:      total,
		Page:       req.Page,
		PageSize:   req.PageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *UserService) GetUserByID(userID string) (*UserView, error) {
	if _, err := paymentpkg.GetDefaultPlanGroup(nil); err != nil {
		return nil, err
	}
	var user UserView
	result := buildUsersWithPlanGroupSelect(db.DB.Model(&models.User{})).
		Where("users.id = ?", userID).
		First(&user)
	if result.Error != nil {
		return nil, ErrUserNotFound
	}
	user.markUsingDefaultPlanGroup()
	return &user, nil
}

func (s *UserService) UpdateUserByAdmin(userID string, req *AdminUpdateUserRequest) (*UserView, error) {
	if req == nil {
		return nil, ErrRequestInvalid
	}
	if req.Email == nil && req.IsActive == nil && req.PlanGroup == nil && req.ExpiresAt == nil && !req.ClearExpiresAt {
		return nil, ErrUpdateFieldsRequired
	}
	if req.ClearExpiresAt && req.ExpiresAt != nil {
		return nil, ErrClearExpiresAtConflict
	}

	tx := db.DB.Begin()
	if tx.Error != nil {
		return nil, ErrUserUpdateFailed
	}

	var user models.User
	result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", userID).First(&user)
	if result.Error != nil {
		tx.Rollback()
		return nil, ErrUserNotFound
	}

	needSyncEmbyPolicy := adminUpdateChangesEmbyPolicy(req)
	oldEffectivePlanGroup, err := paymentpkg.ResolveEffectivePlanGroupKey(tx, user.PlanGroup)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	if req.Email != nil {
		email := strings.TrimSpace(*req.Email)
		if email == "" {
			tx.Rollback()
			return nil, ErrEmailRequired
		}
		if _, err := mail.ParseAddress(email); err != nil {
			tx.Rollback()
			return nil, ErrEmailInvalid
		}
		user.Email = email
	}

	if req.IsActive != nil {
		user.IsActive = *req.IsActive
	}

	if req.PlanGroup != nil {
		planGroup, err := normalizePlanGroupUpdate(*req.PlanGroup)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		if _, err := paymentpkg.GetPlanGroupByKey(tx, planGroup); err != nil {
			tx.Rollback()
			return nil, err
		}
		user.PlanGroup = &planGroup
	}

	if req.ClearExpiresAt {
		user.ExpiresAt = nil
	} else if req.ExpiresAt != nil {
		expiresAt, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			tx.Rollback()
			return nil, ErrExpiresAtFormatInvalid
		}
		expiresAtUTC := expiresAt.UTC()
		user.ExpiresAt = &expiresAtUTC
	}

	updates := map[string]interface{}{
		"email":      user.Email,
		"is_active":  user.IsActive,
		"plan_group": user.PlanGroup,
		"expires_at": user.ExpiresAt,
	}

	if err := tx.Model(&models.User{}).
		Where("id = ?", user.ID).
		Updates(updates).Error; err != nil {
		tx.Rollback()
		if isUserUniqueViolation(err, "email") {
			return nil, ErrEmailAlreadyExists
		}
		return nil, ErrUserUpdateFailed
	}

	newEffectivePlanGroup, err := paymentpkg.ResolveEffectivePlanGroupKey(tx, user.PlanGroup)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	if oldEffectivePlanGroup != newEffectivePlanGroup {
		expiredSessionIDs, err := paymentpkg.PendingStripeSessionIDsForUser(tx, user.ID)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		expiredCount, err := paymentpkg.ExpirePendingPaymentsForUser(tx, user.ID)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		log.Printf("[User] 用户有效套餐分组变更，已收口待支付订单: userID=%s oldPlanGroup=%s newPlanGroup=%s expiredCount=%d",
			user.ID, oldEffectivePlanGroup, newEffectivePlanGroup, expiredCount)
		if err := tx.Commit().Error; err != nil {
			return nil, ErrUserUpdateFailed
		}
		if err := s.syncEmbyPolicy(&user, "admin_plan_group_update"); err != nil {
			return nil, err
		}
		paymentpkg.ExpireStripeCheckoutSessions(expiredSessionIDs)
		return s.GetUserByID(userID)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, ErrUserUpdateFailed
	}
	if needSyncEmbyPolicy {
		if err := s.syncEmbyPolicy(&user, "admin_user_update"); err != nil {
			return nil, err
		}
	}

	return s.GetUserByID(userID)
}

func (s *UserService) ExtendExpiry(userID string, days int) (*UserView, error) {
	user, err := s.findUserByID(userID)
	if err != nil {
		return nil, normalizeUserLookupError(err)
	}

	var newExpiry time.Time
	now := time.Now().UTC()
	if user.ExpiresAt == nil || user.ExpiresAt.Before(now) {
		newExpiry = now.AddDate(0, 0, days)
	} else {
		newExpiry = user.ExpiresAt.AddDate(0, 0, days)
	}

	user.ExpiresAt = &newExpiry
	if err := db.DB.Model(&models.User{}).
		Where("id = ?", user.ID).
		Updates(map[string]interface{}{
			"expires_at": user.ExpiresAt,
		}).Error; err != nil {
		return nil, err
	}
	if err := s.syncEmbyPolicy(user, "user_expiry_extended"); err != nil {
		return nil, err
	}

	return s.GetUserByID(userID)
}

func (s *UserService) ToggleUserStatus(userID string) (*UserView, error) {
	user, err := s.findUserByID(userID)
	if err != nil {
		return nil, normalizeUserLookupError(err)
	}

	user.IsActive = !user.IsActive
	if err := db.DB.Model(&models.User{}).
		Where("id = ?", user.ID).
		Updates(map[string]interface{}{
			"is_active": user.IsActive,
		}).Error; err != nil {
		return nil, err
	}

	return s.GetUserByID(userID)
}

// adminUpdateChangesEmbyPolicy 判断管理员编辑请求是否修改了 Emby Policy 直接依赖的本地字段。
// users.is_active 只控制 Ember 本地登录，不参与 Emby IsDisabled 计算。
func adminUpdateChangesEmbyPolicy(req *AdminUpdateUserRequest) bool {
	if req == nil {
		return false
	}
	return req.ClearExpiresAt || req.ExpiresAt != nil
}

func (s *UserService) DeleteUser(userID string) error {
	user, err := s.findUserByID(userID)
	if err != nil {
		return normalizeUserLookupError(err)
	}

	if user.EmbyID != "" {
		embyService := embyint.GetSharedService()
		if err := embyService.DeleteUser(user.EmbyID); err != nil {
			return errors.New("删除用户失败：" + err.Error())
		}
	}

	if err := db.DB.Delete(&user).Error; err != nil {
		return err
	}

	return nil
}

func normalizeUserLookupError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrUserNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrUserNotFound
	}
	return err
}
