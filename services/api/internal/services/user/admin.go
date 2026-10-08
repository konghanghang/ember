package user

import (
	"context"
	"errors"
	configpkg "github.com/konghang/ember/backend/internal/config"
	entitlementpkg "github.com/konghang/ember/backend/internal/services/entitlement"
	"github.com/oklog/ulid/v2"
	"log"
	"net/mail"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	embytokenpkg "github.com/konghang/ember/backend/internal/services/embytoken"
	paymentpkg "github.com/konghang/ember/backend/internal/services/payment"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const embyAdminPolicyProtectionText = "There must be at least one user in the system with administrative access"

// AdminUpdateUserRequest 管理员更新用户请求
type AdminUpdateUserRequest struct {
	ExtendDays     *int    `json:"extendDays" binding:"omitempty,min=1"`
	OperationID    string  `json:"operationId" binding:"omitempty,max=64"`
	Email          *string `json:"email"`
	IsActive       *bool   `json:"isActive"`
	PlanGroup      *string `json:"planGroup"`
	ExpiresAt      *string `json:"expiresAt"`
	ClearExpiresAt bool    `json:"clearExpiresAt"`
}

// GetUsersRequest 获取用户列表请求
type GetUsersRequest struct {
	Page             int    `form:"page" binding:"omitempty,min=1"`
	PageSize         int    `form:"pageSize" binding:"omitempty,min=1"`
	Search           string `form:"search"`
	IsActive         *bool  `form:"isActive"`
	ExpiresAfter     string `form:"expiresAfter"`
	EmbyStatus       string `form:"embyStatus"`
	PlanGroup        string `form:"planGroup"`
	EntitlementGroup string `form:"entitlementGroup"`
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
				WHEN COALESCE(users."emby_id", '') <> ''
				  AND users."applied_media_library_template_version" < COALESCE(explicit_pg.media_library_template_version, default_pg.media_library_template_version)
				THEN 'out_of_sync'
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

// GetUsers combines current-group and active-holding filters without changing the user's access projection.
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

	if strings.TrimSpace(req.EntitlementGroup) != "" {
		group, err := normalizePlanGroupStrict(req.EntitlementGroup)
		if err != nil {
			return nil, err
		}
		// Capture one instant for both count and page; EXISTS never duplicates a user with multiple holdings.
		query = query.Where(`EXISTS (
			SELECT 1 FROM user_entitlements holdings
			WHERE holdings.user_id = users.id AND holdings.plan_group = ?
			AND holdings.watch_retention_invalidated_at IS NULL
            AND ((holdings.validity_type = ? AND holdings.expires_at IS NULL)
				OR (holdings.validity_type = ? AND holdings.expires_at > ?))
		)`, group, entitlementpkg.Permanent, entitlementpkg.Duration, time.Now())
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
	return s.UpdateUserByAdminWithContext(context.Background(), userID, req, "system:user-service")
}

// UpdateUserByAdminWithContext preserves the existing transactional admin edit
// while revoking old mappings before any explicit is_active assignment.
// Date inputs without an offset use CRON_TIMEZONE; an explicit extension shares the transaction
// with profile edits and reuses its operation key to prevent double extension on retries.
func (s *UserService) UpdateUserByAdminWithContext(ctx context.Context, userID string, req *AdminUpdateUserRequest, operatorID string) (*UserView, error) {
	if req == nil {
		return nil, ErrRequestInvalid
	}
	if req.ExtendDays != nil && (*req.ExtendDays < 1 || strings.TrimSpace(req.OperationID) == "" || len(req.OperationID) > 64 || req.PlanGroup != nil || req.ExpiresAt != nil || req.ClearExpiresAt) {
		return nil, ErrRequestInvalid
	}
	var extensionLocation *time.Location
	if req.ExtendDays != nil {
		extensionLocation = configpkg.LoadConfiguredTimezone()
	}
	if req.Email == nil && req.IsActive == nil && req.PlanGroup == nil && req.ExpiresAt == nil && !req.ClearExpiresAt && req.ExtendDays == nil {
		return nil, ErrUpdateFieldsRequired
	}
	if req.ClearExpiresAt && req.ExpiresAt != nil {
		return nil, ErrClearExpiresAtConflict
	}
	var requestedExpiry *time.Time
	if req.ExpiresAt != nil {
		parsed, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			parsed, err = parseAdminExpiryInput(*req.ExpiresAt, configpkg.LoadConfiguredTimezone())
		}
		if err != nil {
			return nil, ErrExpiresAtFormatInvalid
		}
		parsed = parsed.UTC()
		requestedExpiry = &parsed
	}
	// A group-only edit transfers the existing grant without changing its term.
	transferGroup := req.PlanGroup != nil && req.ExpiresAt == nil && !req.ClearExpiresAt

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
	originalGroup, originalExpiry := user.PlanGroup, user.ExpiresAt

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
	} else if requestedExpiry != nil {
		user.ExpiresAt = requestedExpiry
	}

	if transferGroup {
		if user.IsAdmin() || originalGroup == nil {
			tx.Rollback()
			return nil, ErrRequestInvalid
		}
		targetGroup := *user.PlanGroup
		user.PlanGroup = originalGroup
		if targetGroup != *originalGroup {
			actor := operatorID
			if actor == "" {
				actor = "admin:api-key"
			}
			if err := entitlementpkg.TransferLocked(tx, &user, *originalGroup, targetGroup, "admin:"+ulid.Make().String(), actor); err != nil {
				tx.Rollback()
				return nil, err
			}
		}
	} else if req.PlanGroup != nil || req.ExpiresAt != nil || req.ClearExpiresAt {
		if user.IsAdmin() {
			tx.Rollback()
			return nil, ErrRequestInvalid
		}
		target := entitlementpkg.Holding{ValidityType: entitlementpkg.Duration, ExpiresAt: user.ExpiresAt}
		if user.PlanGroup == nil {
			tx.Rollback()
			return nil, ErrRequestInvalid
		}
		target.PlanGroup = *user.PlanGroup
		if target.ExpiresAt == nil {
			target.ValidityType = entitlementpkg.Permanent
		}
		user.PlanGroup, user.ExpiresAt = originalGroup, originalExpiry
		actor := operatorID
		if actor == "" {
			actor = "admin:api-key"
		}
		if err := entitlementpkg.AdjustLocked(tx, &user, target, false, "admin:"+ulid.Make().String(), actor, time.Now()); err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if req.ExtendDays != nil {
		if user.IsAdmin() || user.PlanGroup == nil {
			tx.Rollback()
			return nil, ErrRequestInvalid
		}
		actor := operatorID
		if actor == "" {
			actor = "admin:api-key"
		}
		key := "admin-edit-extension:" + user.ID + ":" + req.OperationID
		if err := entitlementpkg.GrantLocked(tx, &user, []entitlementpkg.Benefit{{PlanGroup: *user.PlanGroup, ValidityType: entitlementpkg.Duration, DurationDays: *req.ExtendDays}}, key, actor, time.Now(), extensionLocation); err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	updates := map[string]interface{}{
		"email":      user.Email,
		"is_active":  user.IsActive,
		"plan_group": user.PlanGroup,
		"expires_at": user.ExpiresAt,
	}
	if req.IsActive != nil {
		reason := embytokenpkg.RevokeReasonUserDisabled
		if *req.IsActive {
			reason = embytokenpkg.RevokeReasonSecurityRevoke
		}
		count, revokeErr := s.revokeUserTokens(ctx, user.ID, reason, operatorID)
		if revokeErr != nil {
			tx.Rollback()
			return nil, ErrUserTokenRevocation
		}
		log.Printf("[User] 管理员状态赋值前已撤销登录 userID=%s targetActive=%t reason=%s count=%d",
			user.ID, *req.IsActive, reason, count)
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

// ExtendExpiry 为管理员手动续期用户，生产路径在事务内锁定用户行后按最新到期日累加。
func (s *UserService) ExtendExpiry(userID string, days int) (*UserView, error) {
	store := s.extendExpiryStore
	if store == nil {
		store = s.extendExpiryWithDB
	}
	user, err := store(userID, days)
	if err != nil {
		return nil, err
	}
	if err := s.syncEmbyPolicy(user, "user_expiry_extended"); err != nil {
		return nil, err
	}

	return s.GetUserByID(userID)
}

// extendExpiryWithDB 在事务中锁定用户行、基于最新到期日累加有效期并持久化结果。
func (s *UserService) extendExpiryWithDB(userID string, days int) (*models.User, error) {
	location := configpkg.LoadConfiguredTimezone()
	tx := db.DB.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}

	user, err := lockUserForExpiryExtension(tx, userID)
	if err != nil {
		tx.Rollback()
		return nil, normalizeUserLookupError(err)
	}

	now := time.Now()
	if user.PlanGroup == nil {
		tx.Rollback()
		return nil, ErrRequestInvalid
	}
	if err := entitlementpkg.GrantLocked(tx, user, []entitlementpkg.Benefit{{PlanGroup: *user.PlanGroup, ValidityType: entitlementpkg.Duration, DurationDays: days}}, "admin:"+ulid.Make().String(), "admin:legacy-extension", now, location); err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	log.Printf("[User] 管理员续期已提交: userID=%s days=%d newExpiresAt=%s", user.ID, days, user.ExpiresAt.Format(time.RFC3339))
	return user, nil
}

// lockUserForExpiryExtension 读取并锁定管理员续期目标用户，保证累加基准来自已提交的最新到期日。
func lockUserForExpiryExtension(tx *gorm.DB, userID string) (*models.User, error) {
	var user models.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// calculateExtendedExpiry 计算管理员手动续期后的用户到期日；有效账号从原到期日累加，空或已过期账号从当前时间起算。
func calculateExtendedExpiry(now time.Time, currentExpiry *time.Time, days int) time.Time {
	if currentExpiry == nil || currentExpiry.Before(now) {
		return now.AddDate(0, 0, days)
	}
	return currentExpiry.AddDate(0, 0, days)
}

func (s *UserService) ToggleUserStatus(userID string) (*UserView, error) {
	return s.ToggleUserStatusWithContext(context.Background(), userID, "system:user-service")
}

// ToggleUserStatusWithContext revokes all historical mappings before both
// disable and restore transitions so an old Token can never revive on restore.
func (s *UserService) ToggleUserStatusWithContext(ctx context.Context, userID, operatorID string) (*UserView, error) {
	user, err := s.findUserByID(userID)
	if err != nil {
		return nil, normalizeUserLookupError(err)
	}

	user.IsActive = !user.IsActive
	reason := embytokenpkg.RevokeReasonUserDisabled
	if user.IsActive {
		reason = embytokenpkg.RevokeReasonSecurityRevoke
	}
	count, err := s.revokeUserTokens(ctx, user.ID, reason, operatorID)
	if err != nil {
		return nil, ErrUserTokenRevocation
	}
	log.Printf("[User] 用户状态变更前已撤销登录 userID=%s targetActive=%t reason=%s count=%d",
		user.ID, user.IsActive, reason, count)
	if err := s.updateUserActive(user.ID, user.IsActive); err != nil {
		return nil, err
	}

	return s.getUserViewByID(userID)
}

// adminUpdateChangesEmbyPolicy 判断管理员编辑请求是否修改了 Emby Policy 直接依赖的本地字段。
// users.is_active 只控制 Ember 本地登录，不参与 Emby IsDisabled 计算。
func adminUpdateChangesEmbyPolicy(req *AdminUpdateUserRequest) bool {
	if req == nil {
		return false
	}
	return req.PlanGroup != nil || req.ClearExpiresAt || req.ExpiresAt != nil || req.ExtendDays != nil
}

func (s *UserService) DeleteUser(userID string) error {
	return s.DeleteUserWithContext(context.Background(), userID, "system:user-service")
}

// DeleteUserWithContext revokes local Gateway access before any external or
// destructive user deletion step.
func (s *UserService) DeleteUserWithContext(ctx context.Context, userID, operatorID string) error {
	user, err := s.findUserByID(userID)
	if err != nil {
		return normalizeUserLookupError(err)
	}
	count, err := s.revokeUserTokens(ctx, user.ID, embytokenpkg.RevokeReasonUserDeleted, operatorID)
	if err != nil {
		return ErrUserTokenRevocation
	}
	log.Printf("[User] 删除前已撤销用户登录 userID=%s count=%d", user.ID, count)
	if s.revokePersonalP115Account == nil || s.revokePersonalP115Account(ctx, user.ID) != nil {
		return ErrUserP115AccountRevocation
	}
	log.Printf("[User] 删除前已撤销个人 115 账号 userID=%s", user.ID)

	if user.EmbyID != "" {
		if err := s.embyClient().DeleteUser(user.EmbyID); err != nil {
			return errors.New("删除用户失败：" + err.Error())
		}
	}

	if err := s.deleteUserRecord(user); err != nil {
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
